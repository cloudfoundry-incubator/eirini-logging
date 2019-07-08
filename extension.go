package main

import (
	"context"
	"net/http"
	"os"

	"github.com/pkg/errors"

	eirinix "github.com/SUSE/eirinix"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

type Extension struct{ Namespace string }

func getVolume(name, path string) (v1.Volume, v1.VolumeMount) {
	mount := v1.VolumeMount{
		Name:      name,
		MountPath: path,
	}

	vol := v1.Volume{
		Name: name,
	}

	return vol, mount
}

func (ext *Extension) Handle(ctx context.Context, eiriniManager eirinix.Manager, pod *v1.Pod, req types.Request) types.Response {

	if pod == nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.New("No pod could be decoded from the request"))
	}

	config, err := eiriniManager.GetKubeConnection()
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.Wrap(err, "Failed getting the Kube connection"))
	}

	sidecarImage := os.Getenv("DOCKER_SIDECAR_IMAGE")
	if sidecarImage == "" {
		sidecarImage = "splatform/eirini-logging"
	}

	// TODO: Ask the operator to give us a serviceaccount name for a service account with
	// the permission to read logs from any pod in the Eirini namespace instead of this.

	podCopy := pod.DeepCopy()

	// Mount the serviceaccount token in the container
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.Wrap(err, "Failed to create a kube client"))
	}
	serviceAccountHelper := NewServiceAccountMountHelper(kubeClient)
	// TODO: Don't hardcode the "default" service account here, get it from env or something
	serviceAccountVolume, err := serviceAccountHelper.VolumeForServiceAccount("default", ext.Namespace)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.Wrap(err, "Failed to create the serviceaccount token volume"))
	}

	// https://kubernetes.io/docs/tasks/administer-cluster/access-cluster-api/#accessing-the-api-from-a-pod
	serviceAccountVolumeMount := v1.VolumeMount{
		Name:      "serviceaccounttoken",
		ReadOnly:  true,
		MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
	}

	secretsVolumeMount := v1.VolumeMount{
		Name:      "doppler-secrets",
		ReadOnly:  true,
		MountPath: "/secrets",
	}

	secretsVolume := v1.Volume{
		Name: "doppler-secrets",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: os.Getenv("DOPPLER_SECRET"),
				Items: []v1.KeyToPath{
					{Key: "internal-ca-cert", Path: "internal-ca-cert"},
					{Key: "loggregator-forward-cert", Path: "loggregator-forward-cert"},
					{Key: "loggregator-forward-cert-key", Path: "loggregator-forward-cert-key"},
				},
			},
		},
	}

	podCopy.Spec.Volumes = append(podCopy.Spec.Volumes, secretsVolume, serviceAccountVolume)

	sourceType, ok := podCopy.GetLabels()["source_type"]
	//  https://github.com/gdankov/loggregator-ci/blob/eirini/docker-images/fluentd/plugins/loggregator.rb#L46
	if ok && sourceType == "APP" {
		sourceType = "APP/PROC/WEB"
	}

	// FIXME: we assume that the first container is the eirini app
	// FIXME: Don't hardcode scf specific values below
	sidecar := v1.Container{
		Name:            "eirini-logging",
		Image:           sidecarImage,
		Args:            []string{"loggregator"},
		ImagePullPolicy: v1.PullAlways,
		Env: []v1.EnvVar{
			{
				Name:  "LOGGREGATOR_CA_PATH",
				Value: "/secrets/internal-ca-cert",
			},
			{
				Name:  "LOGGREGATOR_CERT_PATH",
				Value: "/secrets/loggregator-forward-cert",
			},
			{
				Name:  "LOGGREGATOR_CERT_KEY_PATH",
				Value: "/secrets/loggregator-forward-cert-key",
			},
			{
				Name:  "LOGGREGATOR_ENDPOINT",
				Value: os.Getenv("LOGGREGATOR_ENDPOINT"),
			},
			{
				Name:  "EIRINI_LOGGREGATOR_SOURCE_ID",
				Value: podCopy.GetLabels()["guid"], // TODO: Handle the case when this is empty
			},
			{
				Name:  "EIRINI_LOGGREGATOR_SOURCE_TYPE",
				Value: sourceType,
			},
			{
				Name:  "EIRINI_LOGGREGATOR_POD_NAME",
				Value: podCopy.GetName(),
			},
			{
				Name:  "EIRINI_LOGGREGATOR_NAMESPACE",
				Value: podCopy.Namespace,
			},
			{
				Name:  "EIRINI_LOGGREGATOR_CONTAINER",
				Value: podCopy.Spec.Containers[0].Name,
			},
			{
				Name: "EIRINI_LOGGREGATOR_CLUSTER",
				// TODO: Is this correct?
				// https://github.com/gdankov/loggregator-ci/blob/eirini/docker-images/fluentd/plugins/loggregator.rb#L54
				Value: podCopy.GetClusterName(),
			},
		},
		// Volumes are mounted for kubeAPI access from the sidecar container
		VolumeMounts: []v1.VolumeMount{secretsVolumeMount, serviceAccountVolumeMount},
	}

	podCopy.Spec.Containers = append(podCopy.Spec.Containers, sidecar)

	return admission.PatchResponse(pod, podCopy)
}

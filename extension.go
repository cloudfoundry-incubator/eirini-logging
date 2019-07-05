package main

import (
	"context"
	"net/http"
	"os"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eirinix "github.com/SUSE/eirinix"
	v1 "k8s.io/api/core/v1"
	rbacapi "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes"
	rbac "k8s.io/client-go/kubernetes/typed/rbac/v1"
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

	rbacClient, err := rbac.NewForConfig(config)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.Wrap(err, "Failed Creating RBAC Client"))
	}

	roleBindingName := "role-binding-" + pod.Name
	roleName := "role-" + pod.Name

	sidecarImage := os.Getenv("DOCKER_SIDECAR_IMAGE")
	if sidecarImage == "" {
		sidecarImage = "splatform/eirini-logging"
	}

	// TODO: Do we need to also create a new service account for each application?
	// If not, any sidecar will be able to read logs from any application
	_, err = rbacClient.Roles(ext.Namespace).Create(&rbacapi.Role{
		TypeMeta:   metav1.TypeMeta{Kind: "Role", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: ext.Namespace, Name: roleName},
		Rules: []rbacapi.PolicyRule{
			{
				ResourceNames: []string{pod.Name},
				Verbs:         []string{"get"},
				Resources:     []string{"pods", "pods/log"},
				APIGroups:     []string{""},
			},

			{
				ResourceNames: []string{roleName},
				Verbs:         []string{"delete"},
				Resources:     []string{"role"},
				APIGroups:     []string{""},
			},
			{
				ResourceNames: []string{roleBindingName},
				Verbs:         []string{"delete"},
				Resources:     []string{"rolebinding"},
				APIGroups:     []string{""},
			},
		}})
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.Wrap(err, "Failed Creating RBAC Role "))
	}

	// TODO: Don't use the "default" service account here but require a service account from the operator which
	// will be used for the eirini applications to be able to talk to the kube api.
	_, err = rbacClient.RoleBindings(ext.Namespace).Create(&rbacapi.RoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "RoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: ext.Namespace, Name: roleBindingName},
		Subjects:   []rbacapi.Subject{{Kind: "ServiceAccount", Name: "default", Namespace: ext.Namespace}},
		RoleRef: rbacapi.RoleRef{
			Kind:     "Role",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		}})
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.Wrap(err, "Failed Creating RBAC RoleBinding"))
	}

	podCopy := pod.DeepCopy()

	// TODO: Remove these
	//podCopy.Spec.ServiceAccountName = "default"
	//autoMountServiceAccount := true
	//podCopy.Spec.AutomountServiceAccountToken = &autoMountServiceAccount

	// Mount the serviceaccount token in the container
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.Wrap(err, "Failed to create a kube client"))
	}
	serviceAccountHelper := NewServiceAccountMountHelper(kubeClient)
	// TODO: Don't hardcode the "default" service account here, get it from env or something
	serviceAccountVolume, err := serviceAccountHelper.VolumeForServiceAccount("default", podCopy.GetNamespace())
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

	// FIXME:Find a better way to do this
	// If the hook fails for any reason the pod gets removed and we keep the role bindings behind
	// We could use e.g. finalizers, preStart hooks that sets the ownership of the pod once it is created

	// Another way to fix it is to create a postStart hook that sets up a parent relation dependency for garbage collection, see:
	// https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/
	// https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/
	// https://kubernetes.io/docs/tasks/run-application/update-api-object-kubectl-patch/
	// https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks

	// If this way proves to work, its better as we don't have to care if the preStop fails (and of the garbage left behind)

	// FIXME: If we fail to create the pod, the roles stay behind and the next time it will complain that the role already exists
	// FIXME: We don't have kubectl in the sidecar
	sidecar.Lifecycle = &v1.Lifecycle{
		PreStop: &v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"/bin/sh",
					"-c",
					"kubectl delete role " + roleName + " -n " + ext.Namespace + " && " + "kubectl delete rolebinding " + roleBindingName + " -n " + ext.Namespace,
				},
			},
		}}

	podCopy.Spec.Containers = append(podCopy.Spec.Containers, sidecar)

	return admission.PatchResponse(pod, podCopy)
}

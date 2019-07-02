package main

import (
	"context"
	"net/http"
	"os"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eirinix "github.com/SUSE/eirinix"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	rbacapi "k8s.io/api/rbac/v1"
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

func (ext *Extension) Handle(ctx context.Context, eiriniManager eirinix.Manager, pod *corev1.Pod, req types.Request) types.Response {

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
		sidecarImage = "splatform/eirini-logging-sidecar"
	}

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

	// FIXME: we assume that the first container is the eirini app
	sidecar := corev1.Container{
		Name:         "eirini-logging",
		Image:        sidecarImage,
		Args:         []string{"logs", "-f", pod.Name, "-n", ext.Namespace, "-c", podCopy.Spec.Containers[0].Name},
		VolumeMounts: podCopy.Spec.Containers[0].VolumeMounts, // Volumes are mounted for kubeAPI access from the sidecar container
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

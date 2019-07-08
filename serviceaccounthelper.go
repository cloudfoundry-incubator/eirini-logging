package main

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ServiceAccountHelper interface {
	VolumeForServiceAccount(string, string) (v1.Volume, error)
}

// Make compilation fail in case our struct doesn't implement the interface
var _ ServiceAccountHelper = &ServiceAccountMountHelper{}

// ServiceAccountMountHelper implements a helper to mount secrets
// as Container volume mounts
type ServiceAccountMountHelper struct {
	Namespace          string
	ServiceAccountName string
	kubeClient         *kubernetes.Clientset
}

func NewServiceAccountMountHelper(kubeClient *kubernetes.Clientset) ServiceAccountHelper {
	return &ServiceAccountMountHelper{
		kubeClient: kubeClient,
	}
}

func (samh *ServiceAccountMountHelper) VolumeForServiceAccount(serviceAccount string, namespace string) (v1.Volume, error) {
	secret, err := samh.getSecret(serviceAccount, namespace)
	if err != nil {
		return v1.Volume{}, err
	}

	secretsVolume := v1.Volume{
		Name: "serviceaccounttoken",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: secret.GetName(),
				Items: []v1.KeyToPath{
					{Key: "token", Path: "token"},
					{Key: "ca.crt", Path: "ca.crt"},
					{Key: "namespace", Path: "namespace"},
				},
			},
		},
	}

	return secretsVolume, nil
}

// TODO: Do we need special permissions to get the secrets?
func (samh *ServiceAccountMountHelper) getSecret(serviceAccount string, namespace string) (v1.Secret, error) {
	secrets, err := samh.kubeClient.CoreV1().Secrets(namespace).List(metav1.ListOptions{
		FieldSelector: "type=" + string(v1.SecretTypeServiceAccountToken),
	})

	if err != nil {
		return v1.Secret{}, nil
	}

	for _, v := range secrets.Items {
		associatedServiceAccount, ok := v.GetAnnotations()["kubernetes.io/service-account.name"]
		if !ok || associatedServiceAccount != serviceAccount {
			continue
		}

		return v, nil
	}

	return v1.Secret{}, nil
}

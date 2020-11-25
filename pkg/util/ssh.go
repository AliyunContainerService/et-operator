package util

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"

	"crypto/rsa"
	"k8s.io/klog"
)

const (
	// SSHPrivateKey private key
	SSHPrivateKey = "id_rsa"

	// SSHPublicKey public key
	SSHPublicKey = "id_rsa.pub"

	// SSHAuthorizedKeys authkey
	SSHAuthorizedKeys = "authorized_keys"

	// SSHConfig  ssh conf
	SSHConfig = "config"

	// SSHAbsolutePath ssh abs path
	SSHAbsolutePath = "/root/.ssh"

	// SSHRelativePath ssh rel path
	SSHRelativePath = ".ssh"
)

func GenerateRsaKey() (map[string][]byte, error) {
	bitSize := 1024

	privateKey, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		klog.Errorf("rsa generateKey err: %v", err)
		return nil, err
	}

	// id_rsa
	privBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyBytes := pem.EncodeToMemory(&privBlock)

	// id_rsa.pub
	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		klog.Errorf("ssh newPublicKey err: %v", err)
		return nil, err
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(publicRsaKey)

	data := make(map[string][]byte)
	data[SSHPrivateKey] = privateKeyBytes
	data[SSHPublicKey] = publicKeyBytes
	data[SSHConfig] = []byte(generateSSHConfig())

	return data, nil
}

func generateSSHConfig() string {
	config := "StrictHostKeyChecking no\nUserKnownHostsFile /dev/null\n"
	// alias hostname
	//config += "Host " + hostName + "\n"
	// host ip addr or domain
	//config += "  HostName " + hostName + "." + subdomain + "\n"

	return config
}

func MountRsaKey(pod *corev1.Pod, secretName string) {
	sshVolume := corev1.Volume{
		Name: secretName,
	}
	var mode int32 = 0600
	sshVolume.Secret = &corev1.SecretVolumeSource{
		SecretName: secretName,
		Items: []corev1.KeyToPath{
			{
				Key:  SSHPrivateKey,
				Path: SSHRelativePath + "/" + SSHPrivateKey,
			},
			{
				Key:  SSHPublicKey,
				Path: SSHRelativePath + "/" + SSHPublicKey,
			},
			{
				Key:  SSHPublicKey,
				Path: SSHRelativePath + "/" + SSHAuthorizedKeys,
			},
			{
				Key:  SSHConfig,
				Path: SSHRelativePath + "/" + SSHConfig,
			},
		},
		DefaultMode: &mode,
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, sshVolume)
	for i, c := range pod.Spec.Containers {
		vm := corev1.VolumeMount{
			MountPath: SSHAbsolutePath,
			SubPath:   SSHRelativePath,
			Name:      secretName,
		}

		pod.Spec.Containers[i].VolumeMounts = append(c.VolumeMounts, vm)
	}
}

package helm

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

var (
	basicAuthSecretFixture = corev1.Secret{
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("password"),
		},
	}
	tlsSecretFixture = corev1.Secret{
		Data: map[string][]byte{
			"certFile": []byte(`fixture`),
			"keyFile":  []byte(`fixture`),
			"caFile":   []byte(`fixture`),
		},
	}
)

func TestClientOptionsFromSecret(t *testing.T) {
	tests := []struct {
		name    string
		secrets []corev1.Secret
	}{
		{"basic auth", []corev1.Secret{basicAuthSecretFixture}},
		{"TLS", []corev1.Secret{tlsSecretFixture}},
		{"basic auth and TLS", []corev1.Secret{basicAuthSecretFixture, tlsSecretFixture}},
		{"empty", []corev1.Secret{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := corev1.Secret{Data: map[string][]byte{}}
			for _, s := range tt.secrets {
				for k, v := range s.Data {
					secret.Data[k] = v
				}
			}
			got, cleanup, err := ClientOptionsFromSecret(secret)
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil {
				t.Errorf("ClientOptionsFromSecret() error = %v", err)
				return
			}
			if len(got) != len(tt.secrets) {
				t.Errorf("ClientOptionsFromSecret() options = %v, expected = %v", got, len(tt.secrets))
			}
		})
	}
}

func TestBasicAuthFromSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  corev1.Secret
		modify  func(secret *corev1.Secret)
		wantErr bool
		wantNil bool
	}{
		{"username and password", basicAuthSecretFixture, nil, false, false},
		{"without username", basicAuthSecretFixture, func(s *corev1.Secret) { delete(s.Data, "username") }, true, true},
		{"without password", basicAuthSecretFixture, func(s *corev1.Secret) { delete(s.Data, "password") }, true, true},
		{"empty", corev1.Secret{}, nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := tt.secret.DeepCopy()
			if tt.modify != nil {
				tt.modify(secret)
			}
			got, err := BasicAuthFromSecret(*secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("BasicAuthFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && got != nil {
				t.Error("BasicAuthFromSecret() != nil")
				return
			}
		})
	}
}

func TestTLSClientConfigFromSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  corev1.Secret
		modify  func(secret *corev1.Secret)
		wantErr bool
		wantNil bool
	}{
		{"certFile, keyFile and caFile", tlsSecretFixture, nil, false, false},
		{"without certFile", tlsSecretFixture, func(s *corev1.Secret) { delete(s.Data, "certFile") }, true, true},
		{"without keyFile", tlsSecretFixture, func(s *corev1.Secret) { delete(s.Data, "keyFile") }, true, true},
		{"without caFile", tlsSecretFixture, func(s *corev1.Secret) { delete(s.Data, "caFile") }, true, true},
		{"empty", corev1.Secret{}, nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := tt.secret.DeepCopy()
			if tt.modify != nil {
				tt.modify(secret)
			}
			got, cleanup, err := TLSClientConfigFromSecret(*secret)
			if cleanup != nil {
				defer cleanup()
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("TLSClientConfigFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && got != nil {
				t.Error("TLSClientConfigFromSecret() != nil")
				return
			}
		})
	}
}

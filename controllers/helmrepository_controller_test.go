package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	sourcev1 "github.com/fluxcd/source-controller/api/v1alpha1"
	"github.com/fluxcd/source-controller/internal/testserver"
)

var _ = Describe("HelmRepositoryReconciler", func() {

	const (
		timeout  = time.Second * 30
		interval = time.Second * 1
	)

	var (
		namespace  *corev1.Namespace
		helmServer *testserver.Helm
		err        error
	)

	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "helm-repository-" + randStringRunes(5)},
		}
		err = k8sClient.Create(context.Background(), namespace)
		Expect(err).NotTo(HaveOccurred(), "failed to create test namespace")
	})

	AfterEach(func() {
		err = k8sClient.Delete(context.Background(), namespace)
		Expect(err).NotTo(HaveOccurred(), "failed to delete test namespace")
	})

	Context("HelmRepository", func() {
		It("Should create successfully", func() {
			helmServer, err = makeHelmRepoSrv()
			Expect(err).NotTo(HaveOccurred(), "failed to setup tmp helm repository server")
			defer os.RemoveAll(helmServer.Root())
			defer helmServer.Stop()
			helmServer.Start()

			Expect(helmServer.GenerateIndex()).Should(Succeed())

			key := types.NamespacedName{
				Name:      "helmrepository-sample-" + randStringRunes(5),
				Namespace: namespace.Name,
			}
			created := &sourcev1.HelmRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: sourcev1.HelmRepositorySpec{
					URL:      helmServer.URL(),
					Interval: metav1.Duration{Duration: interval},
				},
			}
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			got := &sourcev1.HelmRepository{}
			By("Expecting artifact")
			Eventually(func() bool {
				_ = k8sClient.Get(context.Background(), key, got)
				return got.Status.Artifact != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return storage.ArtifactExist(*got.Status.Artifact)
			}).Should(BeTrue())

			By("Updating the chart index")
			// Regenerating the index is sufficient to make the revision change
			Expect(helmServer.GenerateIndex()).Should(Succeed())
			Eventually(func() bool {
				r := &sourcev1.HelmRepository{}
				_ = k8sClient.Get(context.Background(), key, r)
				if r.Status.Artifact == nil {
					return false
				}
				return r.Status.Artifact.Revision != got.Status.Artifact.Revision
			}, timeout, interval).Should(BeTrue())

			updated := &sourcev1.HelmRepository{}
			Expect(k8sClient.Get(context.Background(), key, updated)).Should(Succeed())

			updated.Spec.Interval = metav1.Duration{Duration: 60 * time.Second}
			Expect(k8sClient.Update(context.Background(), updated)).Should(Succeed())

			By("Expecting to delete successfully")
			got = &sourcev1.HelmRepository{}
			Eventually(func() error {
				_ = k8sClient.Get(context.Background(), key, got)
				return k8sClient.Delete(context.Background(), got)
			}, timeout, interval).Should(Succeed())

			By("Expecting delete to finish")
			Eventually(func() error {
				r := &sourcev1.HelmRepository{}
				return k8sClient.Get(context.Background(), key, r)
			}).ShouldNot(Succeed())
			Eventually(func() bool {
				return storage.ArtifactExist(*got.Status.Artifact)
			}).ShouldNot(BeTrue())
		})

		It("Should authenticate when basic auth credentials are provided", func() {
			var username, password = "john", "doe"

			helmServer, err = makeHelmRepoSrv()
			Expect(err).NotTo(HaveOccurred(), "failed to setup tmp helm repository server")
			helmServer.WithMiddleware(func(handler http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					u, p, ok := r.BasicAuth()
					if !ok || username != u || password != p {
						w.WriteHeader(401)
						return
					}
					handler.ServeHTTP(w, r)
				})
			})
			defer os.RemoveAll(helmServer.Root())
			defer helmServer.Stop()
			helmServer.Start()
			Expect(helmServer.GenerateIndex()).Should(Succeed())

			secretKey := types.NamespacedName{
				Name:      "helmrepository-auth-" + randStringRunes(5),
				Namespace: namespace.Name,
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretKey.Name,
					Namespace: secretKey.Namespace,
				},
				Data: map[string][]byte{
					"username": []byte(username),
				},
			}
			Expect(k8sClient.Create(context.Background(), secret)).Should(Succeed())

			key := types.NamespacedName{
				Name:      "helmrepository-sample-" + randStringRunes(5),
				Namespace: namespace.Name,
			}
			created := &sourcev1.HelmRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: sourcev1.HelmRepositorySpec{
					URL: helmServer.URL(),
					SecretRef: &corev1.LocalObjectReference{
						Name: secretKey.Name,
					},
					Interval: metav1.Duration{Duration: interval},
				},
			}
			Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())

			By("Expecting missing field error")
			Eventually(func() bool {
				got := &sourcev1.HelmRepository{}
				_ = k8sClient.Get(context.Background(), key, got)
				for _, c := range got.Status.Conditions {
					if c.Reason == sourcev1.AuthenticationFailedReason {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Adding missing field")
			secret.Data["password"] = []byte(password)
			Expect(k8sClient.Update(context.Background(), secret)).Should(Succeed())
			got := &sourcev1.HelmRepository{}
			Eventually(func() bool {
				_ = k8sClient.Get(context.Background(), key, got)
				return got.Status.Artifact != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return storage.ArtifactExist(*got.Status.Artifact)
			}).Should(BeTrue())
		})
	})
})

func makeHelmRepoSrv() (*testserver.Helm, error) {
	tmpDir, err := ioutil.TempDir("", "helm-pkg-")
	if err != nil {
		return nil, fmt.Errorf("failed to create tmp helm pkg dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	pkg := action.NewPackage()
	pkg.Destination = tmpDir
	if _, err = pkg.Run("testdata/helmchart", nil); err != nil {
		return nil, fmt.Errorf("failed to package helm chart: %w", err)
	}
	httpSrv, err := testserver.NewTempHTTPServer(path.Join(tmpDir, "*.tgz"))
	if err != nil {
		return nil, err
	}
	return &testserver.Helm{HTTP: httpSrv}, nil
}

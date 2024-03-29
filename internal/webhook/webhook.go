package webhook

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	admissionregistrationclientv1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
)

// New creates a new Server with the provided options
func New(opts ...Option) *MsmWebhook {
	w := &MsmWebhook{}

	for _, o := range opts {
		o(w)
	}

	return w
}

// Option is a function that acts on a Server to inject Dependencies or configuration
type Option func(w *MsmWebhook)

// UseDeps returns Option that can inject custom dependencies.
func UseDeps(cb func(*Deps)) Option {
	return func(p *MsmWebhook) {
		cb(&p.Deps)
	}
}

// MsmWebhook holds the data structures for the webhook
type MsmWebhook struct {
	Deps

	server       *http.Server
	deserializer runtime.Decoder
	caBundle     []byte
	client       admissionregistrationclientv1.AdmissionregistrationV1Interface
}

// Deps list dependencies for the Server
type Deps struct {
	Log *logrus.Logger
}

// Init initializes the server
func (w *MsmWebhook) Init(ctx context.Context) error {
	var err error
	w.Log.Info("Initializing server")
	defer w.Log.Info("Server successfully initialized")

	runtimeScheme := runtime.NewScheme()
	w.deserializer = serializer.NewCodecFactory(runtimeScheme).UniversalDeserializer()

	// create certificates
	cert := w.selfSignedCert()

	// admission webhook registration
	c, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(c)
	if err != nil {
		return err
	}
	w.client = clientset.AdmissionregistrationV1()

	err = w.Register(ctx)
	if err != nil {
		return err
	}

	// http server and server handler initialization
	w.server = &http.Server{
		Addr: fmt.Sprintf(":%v", defaultPort),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", w.handle)
	w.server.Handler = mux

	return nil
}

func (w *MsmWebhook) Start() error {
	w.Log.Infof("Server successfully started: listening on port 443")

	return w.server.ListenAndServeTLS("", "")
}

func (w *MsmWebhook) Close() {
	defer w.Log.Infof("Server successfully closed")

	_ = w.Unregister(context.Background())
	_ = w.server.Close()
}

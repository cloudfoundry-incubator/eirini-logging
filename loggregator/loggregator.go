package loggregator

import (
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"code.cloudfoundry.org/go-loggregator"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Loggregator struct {
}

func NewLoggregator() *Loggregator {
	return &Loggregator{}
}

type LoggregatorWriter struct {
	SourceID, Platform, SourceInstance string

	KubeClient        *kubernetes.Clientset
	LoggregatorClient *loggregator.IngressClient
}

func (l *LoggregatorWriter) Write(b []byte) (int, error) {

	tlsConfig, err := loggregator.NewIngressTLSConfig(
		os.Getenv("CA_CERT_PATH"),
		os.Getenv("CERT_PATH"),
		os.Getenv("KEY_PATH"),
	)
	if err != nil {
		return 0, err
	}

	loggregatorClient, err := loggregator.NewIngressClient(
		tlsConfig,
		// Temporary make flushing more frequent to be able to debug
		loggregator.WithBatchFlushInterval(3*time.Second),
		loggregator.WithAddr(os.Getenv("LOGGREGATOR_AGENT")),
	)

	if err != nil {
		return 0, err
	}

	log.Println("POD OUTPUT: " + string(b))
	loggregatorClient.EmitLog(string(b),
		loggregator.WithSourceInfo(l.SourceID, l.Platform, l.SourceInstance),
	)

	return len(b), nil
}

func NewLoggregatorWriter(kubeClient *kubernetes.Clientset) *LoggregatorWriter {

	sourceID := os.Getenv("SOURCE_ID")
	if sourceID == "" {
		sourceID = "v2-example-source-id"
	}

	platformID := os.Getenv("PLATFORM_ID")
	if platformID == "" {
		platformID = "platform"
	}
	sourceInstance := os.Getenv("SOURCE_INSTANCE")
	if sourceInstance == "" {
		sourceInstance = "v2-example-source-instance"
	}

	return &LoggregatorWriter{
		SourceID:       sourceID,
		Platform:       platformID,
		SourceInstance: sourceInstance,
		KubeClient:     kubeClient,
	}
}

func (l *LoggregatorWriter) AttachToPodLogs(namespace, pod, container string) error {

	req := l.KubeClient.CoreV1().RESTClient().Get().
		Namespace(namespace).
		Name(pod).
		Resource("pods").
		SubResource("log").
		Param("follow", strconv.FormatBool(true)).
		Param("container", container).
		Param("previous", strconv.FormatBool(false)).
		Param("timestamps", strconv.FormatBool(false))
	readCloser, err := req.Stream()
	if err != nil {
		return err
	}

	defer readCloser.Close()
	_, err = io.Copy(l, readCloser)
	if err != nil {
		return err
	}

	return errors.New("Something went wrong")
}

func (l *Loggregator) Run() error {
	kubeConfig := os.Getenv("KUBECONFIG")
	var restConfig *rest.Config
	var err error
	if kubeConfig == "" {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return err
		}

	} else {
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			return err
		}
	}

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	writer := NewLoggregatorWriter(kubeClient)
	err = writer.AttachToPodLogs(os.Getenv("NAMESPACE"), os.Getenv("POD"), os.Getenv("CONTAINER"))
	if err != nil {
		return err
	}

	//ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	//defer cancel()

	// err = client.EmitEvent(
	// 	ctx,
	// 	"Starting sample V2 Client",
	// 	"This sample V2 client is about to emit 50 log envelopes",
	// )
	// if err != nil {
	// 	log.Fatalf("Failed to emit event: %s", err)
	// }

	// startTime := time.Now()
	// for i := 0; i < 5; i++ {
	// 	client.EmitTimer("loop_times", startTime, time.Now())
	// }

	return nil
}

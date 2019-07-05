package loggregator

import (
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type LoggregatorMeta struct {
	SourceID, InstanceID                               string
	SourceType, PodName, Namespace, Container, Cluster string // Custom tags
}

type Loggregator struct {
	Meta   *LoggregatorMeta
	Writer *LoggregatorWriter
}

type LoggregatorWriter struct {
	Meta              *LoggregatorMeta
	KubeClient        *kubernetes.Clientset
	LoggregatorClient *loggregator.IngressClient
}

func NewLoggregator(m *LoggregatorMeta) *Loggregator {
	return &Loggregator{Meta: m}
}

func (lw *LoggregatorWriter) Envelope(message []byte) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: message,
				Type:    loggregator_v2.Log_OUT,
			},
		},
		SourceId:   lw.Meta.SourceID,
		InstanceId: lw.Meta.InstanceID,
		Tags: map[string]string{
			"source_type": lw.Meta.SourceType,
			"pod_name":    lw.Meta.PodName,
			"namespace":   lw.Meta.Namespace,
			"container":   lw.Meta.Container,
			"cluster":     lw.Meta.Cluster, // ??
		},
		Timestamp: time.Now().Unix() * 1000000000,
	}
}

func (lw *LoggregatorWriter) Write(b []byte) (int, error) {
	tlsConfig, err := loggregator.NewIngressTLSConfig(
		os.Getenv("LOGGREGATOR_CA_PATH"),
		os.Getenv("LOGGREGATOR_CERT_PATH"),
		os.Getenv("LOGGREGATOR_CERT_KEY_PATH"),
	)
	if err != nil {
		return 0, err
	}

	loggregatorClient, err := loggregator.NewIngressClient(
		tlsConfig,
		// Temporary make flushing more frequent to be able to debug
		loggregator.WithBatchFlushInterval(3*time.Second),
		loggregator.WithAddr(os.Getenv("LOGGREGATOR_ENDPOINT")),
	)

	if err != nil {
		return 0, err
	}

	log.Println("POD OUTPUT: " + string(b))

	loggregatorClient.Emit(lw.Envelope(b))

	return len(b), nil
}

func NewLoggregatorWriter(kubeClient *kubernetes.Clientset, meta *LoggregatorMeta) *LoggregatorWriter {
	return &LoggregatorWriter{
		Meta:       meta,
		KubeClient: kubeClient,
	}
}

func (l *Loggregator) AttachToPodLogs(namespace, pod, container string) error {
	req := l.Writer.KubeClient.CoreV1().RESTClient().Get().
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
	_, err = io.Copy(l.Writer, readCloser)
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

	l.Writer = NewLoggregatorWriter(kubeClient, l.Meta)
	err = l.AttachToPodLogs(
		os.Getenv("EIRINI_LOGGREGATOR_NAMESPACE"),
		os.Getenv("EIRINI_LOGGREGATOR_POD_NAME"),
		os.Getenv("EIRINI_LOGGREGATOR_CONTAINER"),
	)
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

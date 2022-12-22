package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grafana/mimir/pkg/mimirtool/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"

	"github.com/healthjoy/mimir-rules-controller/pkg/controller"
	rulesclientset "github.com/healthjoy/mimir-rules-controller/pkg/generated/clientset/versioned"
	rulesinformers "github.com/healthjoy/mimir-rules-controller/pkg/generated/informers/externalversions"
	"github.com/healthjoy/mimir-rules-controller/pkg/metrics"
)

var (
	masterURL  string
	kubeconfig string
	address    string
	config     controller.Config
	mmConf     client.Config
)

func init() {
	// Kubernetes client flags
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")

	// Controller config
	flag.StringVar(&address, "address", ":9000", "The address to expose prometheus metrics and service endpoints.")
	flag.StringVar(&config.ClusterName, "cluster-name", getEnv("CLUSTER_NAME", "default"), "The name of the cluster. Used to identify the cluster in the Mimir API.")
	flag.StringVar(&config.PodName, "pod-name", getEnv("POD_NAME", ""), "The name of the pod")
	flag.StringVar(&config.PodNamespace, "pod-namespace", getEnv("POD_NAMESPACE", ""), "The namespace of the pod")
	flag.StringVar(&config.LeaseLockName, "lease-lock-name", getEnv("LEASE_LOCK_NAME", "mimir-rules-controller"), "The name of the lease lock resource")
	flag.StringVar(&config.LeaseLockNamespace, "lease-lock-namespace", getEnv("LEASE_LOCK_NAMESPACE", ""), "The namespace of the lease lock resource")

	// Mimic client config
	flag.StringVar(&mmConf.User, "mimir-user", getEnv("MIMIR_USER", ""), "The username for the Mimir API")
	flag.StringVar(&mmConf.Key, "mimir-key", getEnv("MIMIR_KEY", ""), "The key for the Mimir API")
	flag.StringVar(&mmConf.Address, "mimir-addr", getEnv("MIMIR_ADDRESS", ""), "The address of the Mimir API")
	flag.StringVar(&mmConf.ID, "mimir-tenant-id", getEnv("MIMIR_TENANT_ID", ""), "The tenant ID for the Mimir API")
	flag.BoolVar(&mmConf.UseLegacyRoutes, "mimir-use-legacy-routes", getEnv("MIMIR_USE_LEGACY_ROUTES", "false") == "true", "Whether to use legacy routes for the Mimir API")
	flag.StringVar(&mmConf.AuthToken, "mimir-auth-token", getEnv("MIMIR_AUTH_TOKEN", ""), "The auth token for the Mimir API")
	flag.StringVar(&mmConf.TLS.CertPath, "mimir-tls-cert-path", getEnv("MIMIR_TLS_CERT_PATH", ""), "The path to the TLS certificate for the Mimir API")
	flag.StringVar(&mmConf.TLS.KeyPath, "mimir-tls-key-path", getEnv("MIMIR_TLS_KEY_PATH", ""), "The path to the TLS key for the Mimir API")
	flag.StringVar(&mmConf.TLS.CAPath, "mimir-tls-ca-path", getEnv("MIMIR_TLS_CA_PATH", ""), "The path to the TLS CA for the Mimir API")
	flag.StringVar(&mmConf.TLS.ServerName, "mimir-server-name", getEnv("MIMIR_SERVER_NAME", ""), "The server name for the Mimir API")
	flag.StringVar(&mmConf.TLS.CipherSuites, "mimir-tls-cipher-suites", getEnv("MIMIR_TLS_CIPHER_SUITES", ""), "The cipher suites for the Mimir API")
	flag.StringVar(&mmConf.TLS.MinVersion, "mimir-tls-min-version", getEnv("MIMIR_TLS_MIN_VERSION", ""), "The minimum TLS version for the Mimir API")
	flag.BoolVar(&mmConf.TLS.InsecureSkipVerify, "mimir-insecure-skip-verify", getEnv("MIMIR_INSECURE_SKIP_VERIFY", "false") == "true", "Whether to skip TLS verification for the Mimir API")
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if config.PodName == "" {
		klog.Fatal("pod-name is required")
	}
	if config.PodNamespace == "" {
		klog.Fatal("pod-namespace is required")
	}
	if config.LeaseLockNamespace == "" {
		config.LeaseLockNamespace = config.PodNamespace
	}

	// set up signals, so we handle the first shutdown signal gracefully
	ctx := contextWithSigterm(context.Background())

	// creates the connection
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	// create the clientset
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	// Create the rules clientset
	rulesClient, err := rulesclientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building rules clientset: %s", err.Error())
	}

	// Create the mimir client
	mimirClient, err := client.New(mmConf)
	if err != nil {
		klog.Fatalf("Error building mimir client: %s", err.Error())
	}

	// Create the rules informer factory
	rulesInformerFactory := rulesinformers.NewSharedInformerFactory(rulesClient, time.Second*30)

	metricServer := metrics.New()
	// Create the ruleController
	ruleController := controller.NewController(config,
		kubeClient, rulesClient, mimirClient,
		rulesInformerFactory.Rulescontroller().V1alpha1().MimirRules(),
		metricServer.Registry,
	)

	// runServer the informer factories to begin populating the informer caches
	rulesInformerFactory.Start(ctx.Done())

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      config.LeaseLockName,
			Namespace: config.LeaseLockNamespace,
		},
		Client: kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      config.Identity(),
			EventRecorder: nil,
		},
	}

	metricServer.Start(ctx, address)

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Name:          fmt.Sprintf("%s/%s", config.LeaseLockNamespace, config.LeaseLockName),
		Lock:          lock,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				// runServer the ruleController
				klog.Info("Started leading")
				if err = ruleController.Run(ctx, 2); err != nil {
					klog.Fatalf("Error running ruleController: %s", err.Error())
				}
			},
			OnStoppedLeading: func() {
				klog.Fatalf("Stopped leading")
			},
			OnNewLeader: func(identity string) {
				if identity == config.Identity() {
					return
				}
				klog.Infof("New leader elected: %s", identity)
			},
		},
	})
}

// contextWithSigterm returns a context that is cancelled when a SIGTERM is received
func contextWithSigterm(ctx context.Context) context.Context {
	ctxWithCancel, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		signalCh := make(chan os.Signal, 2)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

		select {
		case <-signalCh:
		case <-ctx.Done():
		}
	}()

	return ctxWithCancel
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

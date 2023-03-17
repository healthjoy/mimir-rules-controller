package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/mimir/pkg/mimirtool/client"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	kuberr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/healthjoy/mimir-rules-controller/pkg/apis/rulescontroller/v1alpha1"

	clientset "github.com/healthjoy/mimir-rules-controller/pkg/generated/clientset/versioned"
	informers "github.com/healthjoy/mimir-rules-controller/pkg/generated/informers/externalversions/rulescontroller/v1alpha1"
	listers "github.com/healthjoy/mimir-rules-controller/pkg/generated/listers/rulescontroller/v1alpha1"
)

const controllerAgentName = "mimir-rules-controller"

// Config is the configuration for the controller.
type Config struct {
	// ClusterName is the name of the cluster and use as the first part of the
	// Namespace of RuleNamespace in Mimir
	ClusterName string
	// PodName is the name of the pod running the controller.
	PodName string
	// PodNamespace is the namespace of the pod running the controller.
	PodNamespace string
	// LeaseLockName is the name of the lease lock.
	LeaseLockName string
	// LeaseLockNamespace is the namespace of the lease lock.
	LeaseLockNamespace string

	identity string
}

// Identity returns the identity of the controller.
func (c *Config) Identity() string {
	if c.identity == "" {
		c.identity = fmt.Sprintf("%s-%s", c.PodNamespace, c.PodName)
	}
	return c.identity
}

// Controller is the controller implementation for Rule resources
type Controller struct {
	config *Config
	// rulesclientset is a clientset for our own API group
	rulesclientset clientset.Interface

	// Mimir client
	mimirclient *client.MimirClient

	// rulesLister can list/get rules from the shared informer's store
	rulesLister listers.MimirRuleLister
	// rulesSynced returns true if the rules shared informer has been synced at least once
	rulesSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	// syncCounter prometheus counter
	syncCounter prometheus.Counter

	// syncErrorCounter prometheus counter
	syncErrorCounter prometheus.Counter

	// syncHistogram prometheus histogram
	syncHistogram prometheus.Histogram
}

// NewController returns a new rules controller.
func NewController(
	config Config,
	kubeclientset kubernetes.Interface,
	rulesclientset clientset.Interface,
	mimirclient *client.MimirClient,
	ruleinformer informers.MimirRuleInformer,
	reg *prometheus.Registry) *Controller {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		config:         &config,
		rulesclientset: rulesclientset,
		mimirclient:    mimirclient,
		rulesLister:    ruleinformer.Lister(),
		rulesSynced:    ruleinformer.Informer().HasSynced,
		workqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Rules"),
		recorder:       recorder,

		syncCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mimir_rules_controller_sync_total",
			Help: "Total number of syncs",
		}),

		syncErrorCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mimir_rules_controller_sync_errors_total",
			Help: "Total number of sync errors",
		}),

		syncHistogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "mimir_rules_controller_sync_duration_seconds",
			Help: "Sync duration in seconds",
		}),
	}

	reg.MustRegister(controller.syncCounter)
	reg.MustRegister(controller.syncErrorCounter)
	reg.MustRegister(controller.syncHistogram)

	klog.Info("Setting up event handlers")
	_, _ = ruleinformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueRule,
		DeleteFunc: controller.enqueueRule,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueRule(new)
		},
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until ctx is
// cancelled, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(ctx context.Context, threadiness int) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// runServer the informer factories to begin populating the informer caches
	klog.Info("Starting Rules controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(ctx.Done(), c.rulesSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process Rules resources
	for i := 0; i < threadiness; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	klog.Info("Started workers")

	<-ctx.Done()
	klog.Info("Shutting down workers")

	return nil
}

func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

func (c *Controller) processNextItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if err := c.syncHandler(ctx, key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncHandler(ctx context.Context, key string) error {
	startTime := time.Now()

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Rule resource with this namespace/name
	rule, err := c.rulesLister.MimirRules(namespace).Get(name)
	if err != nil {
		// The Rule resource may no longer exist, in which case we stop
		// processing.
		if kuberr.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("rule '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	klog.Info("Check rule generation")
	if condition := apimeta.FindStatusCondition(rule.Status.Conditions, string(v1alpha1.ConditionTypeReady)); condition != nil {
		if rule.Generation-condition.ObservedGeneration <= 1 {
			klog.Info("Rule is up to date")
			return nil
		}
	}

	// Setup defer to update sync metrics
	defer func() {
		c.syncCounter.Inc()
		c.syncHistogram.Observe(time.Since(startTime).Seconds())
		if err != nil {
			c.syncErrorCounter.Inc()
		}
	}()

	if !rule.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rule, v1alpha1.RuleFinalizer) {
			for _, group := range rule.Spec.Groups {
				if err := c.mimirclient.DeleteRuleGroup(ctx, fmt.Sprintf("%s:%s:%s", c.config.ClusterName, namespace, name), group.Name); err != nil {
					runtime.HandleError(fmt.Errorf("error deleting rule group '%s' for rule '%s': %s", group.Name, key, err.Error()))
					return err
				}
			}
			controllerutil.RemoveFinalizer(rule, v1alpha1.RuleFinalizer)
			if _, err := c.rulesclientset.RulescontrollerV1alpha1().MimirRules(rule.Namespace).Update(ctx, rule, metav1.UpdateOptions{}); err != nil {
				runtime.HandleError(fmt.Errorf("error removing finalizer from rule '%s': %s", key, err.Error()))
				return err
			}
		}
		klog.Infof("Rule '%s' deleted", key)
		return nil
	}

	klog.Info("Setup deferred function")
	defer func() {
		if _, dErr := c.rulesclientset.RulescontrollerV1alpha1().MimirRules(rule.Namespace).Update(ctx, rule, metav1.UpdateOptions{}); dErr != nil {
			dErr = fmt.Errorf("error updating rule status: %s", dErr.Error())
			runtime.HandleError(dErr)
			if err != nil {
				err = fmt.Errorf("exit with %s; update error %s", err, dErr)
			} else {
				err = dErr
			}
		}
		klog.Info("Updated rule status")
	}()

	if !controllerutil.ContainsFinalizer(rule, v1alpha1.RuleFinalizer) {
		controllerutil.AddFinalizer(rule, v1alpha1.RuleFinalizer)
		return nil
	}

	// Do something with the rule here
	klog.Info("Processing rule")
	mimirRuleNs, err := rule.Spec.GetMimirRuleNamespace(fmt.Sprintf("%s:%s:%s", c.config.ClusterName, rule.Namespace, rule.Name))
	if err != nil {
		err := fmt.Errorf("error getting mimir rule namespace: %w", err)
		readyCondition := metav1.Condition{
			Type:               string(v1alpha1.ConditionTypeFailed),
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "Error",
			Message:            err.Error(),
			ObservedGeneration: rule.Generation,
		}
		apimeta.SetStatusCondition(&rule.Status.Conditions, readyCondition)
		runtime.HandleError(err)
		return err
	}
	if errs := mimirRuleNs.Validate(); len(errs) > 0 {
		err := fmt.Errorf("validation err: %w", errors.Join(errs...))
		runtime.HandleError(fmt.Errorf("rule '%s' in work queue has invalid rules: %w", key, err))
		readyCondition := metav1.Condition{
			Type:               string(v1alpha1.ConditionTypeFailed),
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "Error",
			Message:            err.Error(),
			ObservedGeneration: rule.Generation,
		}
		apimeta.SetStatusCondition(&rule.Status.Conditions, readyCondition)
		runtime.HandleError(err)
		return err
	}
	if _, _, err = mimirRuleNs.LintExpressions("mimir"); err != nil {
		err := fmt.Errorf("rule '%s' in work queue has invalid expressions: %s", key, err)
		readyCondition := metav1.Condition{
			Type:               string(v1alpha1.ConditionTypeFailed),
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "Error",
			Message:            err.Error(),
			ObservedGeneration: rule.Generation,
		}
		apimeta.SetStatusCondition(&rule.Status.Conditions, readyCondition)
		runtime.HandleError(err)
		return err
	}
	klog.Info("Creating rule")
	for _, group := range mimirRuleNs.Groups {
		err := c.mimirclient.CreateRuleGroup(ctx, mimirRuleNs.Namespace, group)
		if err != nil {
			err := fmt.Errorf("error creating rule group: %w", err)
			readyCondition := metav1.Condition{
				Type:               string(v1alpha1.ConditionTypeFailed),
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "Error",
				Message:            err.Error(),
				ObservedGeneration: rule.Generation,
			}
			apimeta.SetStatusCondition(&rule.Status.Conditions, readyCondition)
			runtime.HandleError(err)
			return err
		}
	}

	klog.Info("Rule created, updating status")
	readyCondition := metav1.Condition{
		Type:               string(v1alpha1.ConditionTypeReady),
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "Success",
		Message:            "Rule is ready",
		ObservedGeneration: rule.Generation,
	}
	apimeta.SetStatusCondition(&rule.Status.Conditions, readyCondition)

	klog.Info("Done processing rule")
	return nil
}

func (c *Controller) enqueueRule(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

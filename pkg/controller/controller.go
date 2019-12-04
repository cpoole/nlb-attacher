package controller

import (
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/birdrides/nlb-attacher/pkg/aws"
	"github.com/birdrides/nlb-attacher/pkg/config"
	"github.com/birdrides/nlb-attacher/pkg/event"
	"github.com/birdrides/nlb-attacher/pkg/handlers"
)

// Controller - the primary struct responsible for all cluster actions
type Controller struct {
	clientset       kubernetes.Interface
	queue           workqueue.RateLimitingInterface
	informer        cache.SharedIndexInformer
	eventHandler    handlers.Handler
	config          config.Config
	serverStartTime time.Time
}

const maxRetries int = 5
const enableLabelValue string = "nlb-attacher.bird.co/enabled"
const targetGroupAnnotationKey string = "nlb-attacher.bird.co/target-groups"

// Init - initialize the primary controller
func (controller *Controller) Init(config *config.Config) {
	//initialize kubernetes client and api handler
	controller.clientset = returnK8sClient()
	api := controller.clientset.CoreV1()

	//generate our set of filtering options
	//TODO: use namespace option from config to restrict
	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", enableLabelValue),
	}

	listFunc := func(innerListOptions metav1.ListOptions) (runtime.Object, error) {
		return api.Pods(metav1.NamespaceAll).List(listOptions)
	}

	watchFunc := func(innerListOptions metav1.ListOptions) (watch.Interface, error) {
		return api.Pods(metav1.NamespaceAll).Watch(listOptions)
	}

	listWatcher := cache.ListWatch{
		ListFunc:  listFunc,
		WatchFunc: watchFunc,
	}

	controller.informer = cache.NewSharedIndexInformer(
		&listWatcher,
		&v1.Pod{},
		time.Second*60,
		cache.Indexers{},
	)

	//Initialize AWS context by fetching all target groups and ELBS
	controller.eventHandler = new(aws.Handler)
	controller.eventHandler.Init(targetGroupAnnotationKey, enableLabelValue)

	controller.configureController() //controller.clientset, controller.eventHandler, informer)
	stopCh := make(chan struct{})
	defer close(stopCh)

	go controller.Run(stopCh)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm
}

//TODO: better combine/split/refactor this and the Init method
func (controller *Controller) configureController() {
	controller.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var newEvent event.Event
	var err error
	controller.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.Key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.EventType = "create"
			log.WithField("pkg", "pod").Infof("Processing add to: %s", newEvent.Key)
			if err == nil {
				controller.queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			newEvent.Key, err = cache.MetaNamespaceKeyFunc(new)
			newEvent.EventType = "update"
			log.WithField("pkg", "pod").Infof("Processing update to %s", newEvent.Key)
			if err == nil {
				controller.queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			newEvent.Key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			newEvent.EventType = "delete"
			newEvent.Namespace = GetObjectMetaData(obj).Namespace
			log.WithField("pkg", "pod").Infof("Processing delete to %s", newEvent.Key)
			if err == nil {
				controller.queue.Add(newEvent)
			}
		},
	})
}

// Run starts the controller
func (controller *Controller) Run(stopCh <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer controller.queue.ShutDown()

	log.Info("Starting nlb-attacher")
	controller.serverStartTime = time.Now().Local()

	go controller.informer.Run(stopCh)

	// wait for the caches to synchronize before starting the worker
	if !cache.WaitForCacheSync(stopCh, controller.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	log.Info("nlb-attacher synced and ready")

	// runWorker will loop until "something bad" happens.  The .Until will
	// then rekick the worker after one second
	wait.Until(controller.runWorker, time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (controller *Controller) HasSynced() bool {
	return controller.informer.HasSynced()
}

// LastSyncResourceVersion is required for the cache.Controller interface.
func (controller *Controller) LastSyncResourceVersion() string {
	return controller.informer.LastSyncResourceVersion()
}

func (controller *Controller) runWorker() {
	// processNextWorkItem will automatically wait until there's work available
	for controller.processNextItem() {
		// continue looping
	}
}

// processNextWorkItem deals with one key off the queue.  It returns false
// when it's time to quit.
func (controller *Controller) processNextItem() bool {
	// pull the next work item from queue.  It should be a key we use to lookup
	// something in a cache
	newEvent, quit := controller.queue.Get()
	if quit {
		return false
	}

	// you always have to indicate to the queue that you've completed a piece of
	// work
	defer controller.queue.Done(newEvent)

	// do your work on the key.
	err := controller.processItem(newEvent.(event.Event))

	if err == nil {
		// No error, tell the queue to stop tracking history
		controller.queue.Forget(newEvent)
	} else if controller.queue.NumRequeues(newEvent) < maxRetries {
		log.Errorf("Error processing %s (will retry): %v", newEvent, err)
		// requeue the item to work on later
		controller.queue.AddRateLimited(newEvent)
	} else {
		// err != nil and too many retries
		log.Errorf("Error processing %s (giving up): %v", newEvent, err)
		controller.queue.Forget(newEvent)
		//TODO: calling this sends the main control loop into a stall. investigate why
		//utilruntime.HandleError(err)
	}

	return true
}

func (controller *Controller) processItem(newEvent event.Event) error {
	log.Debugf("Handle event: %v", newEvent)
	obj, _, err := controller.informer.GetIndexer().GetByKey(newEvent.Key)
	if err != nil {
		return fmt.Errorf("Error fetching object with key %s from store: %v", newEvent.Key, err)
	}

	objectMeta := GetObjectMetaData(obj)

	currPod, typePod := obj.(*v1.Pod)

	// process events based on its type
	switch newEvent.EventType {
	case "create":
		if typePod {
			if controller.config.GetOnlyNewPods() {
				if objectMeta.CreationTimestamp.Sub(controller.serverStartTime).Seconds() > 0 {
					controller.eventHandler.PodCreated(currPod)
				}
			} else {
				log.Debug("inside create event")
				controller.eventHandler.PodCreated(currPod)
			}
			return nil
		}
		log.Debug(reflect.TypeOf(obj))
		return fmt.Errorf("Returned object is not of type Pod: %v", obj)

	case "update":
		/* TODOs
		- enahace update event processing to send statsd alert about what changed.
		*/
		if typePod {

			controller.eventHandler.PodUpdated(currPod, currPod)
			return nil
		}
		log.Debug(reflect.TypeOf(obj))
		return fmt.Errorf("Returned object is not of type Pod: %v", obj)

	case "delete":
		//TODO: handle DeletedFinalStateUnknown
		//TODO: final deletion event simply gives us the pod name and the fact that it's been deleted
		//this is not enough to call the handler deletion func
		//controller.eventHandler.PodDeleted(currPod)
		log.Warn("We do not currently handle the final deletion event. you might be accumulating dead pods")
		return nil
	}
	return nil
}

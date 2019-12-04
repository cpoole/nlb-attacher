package controller

import (
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	//"k8s.io/apimachinery/pkg/api/resource"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	apps_v1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	api_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func returnK8sClient() *kubernetes.Clientset {
	var config *rest.Config
	var err error

	if !fileExists("/var/run/secrets/kubernetes.io/serviceaccount/token") {
		kubeconfig := filepath.Join(
			os.Getenv("HOME"), ".kube", "config",
		)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Fatal(err)
		}
	}

	// Create an rest client not targeting specific API version
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	return clientset
}

//func keysFromStringMap(input map[string][]targetGroupPodInfo) []string {
//	localSlice := make([]string, 0)
//	for key := range input {
//		localSlice = append(localSlice, key)
//	}
//
//	return localSlice
//}
//
//func pointerKeysFromStringMap(input map[string][]targetGroupPodInfo) []*string {
//	localSlice := make([]*string, 0)
//	for key := range input {
//		local := key
//		localSlice = append(localSlice, &local)
//	}
//
//	return localSlice
//}

// GetObjectMetaData returns metadata of a given k8s object
func GetObjectMetaData(obj interface{}) meta_v1.ObjectMeta {

	var objectMeta meta_v1.ObjectMeta

	switch object := obj.(type) {
	case *apps_v1.Deployment:
		objectMeta = object.ObjectMeta
	case *api_v1.ReplicationController:
		objectMeta = object.ObjectMeta
	case *apps_v1.ReplicaSet:
		objectMeta = object.ObjectMeta
	case *apps_v1.DaemonSet:
		objectMeta = object.ObjectMeta
	case *api_v1.Service:
		objectMeta = object.ObjectMeta
	case *api_v1.Pod:
		objectMeta = object.ObjectMeta
	case *batch_v1.Job:
		objectMeta = object.ObjectMeta
	case *api_v1.PersistentVolume:
		objectMeta = object.ObjectMeta
	case *api_v1.Namespace:
		objectMeta = object.ObjectMeta
	case *api_v1.Secret:
		objectMeta = object.ObjectMeta
	case *ext_v1beta1.Ingress:
		objectMeta = object.ObjectMeta
	}
	return objectMeta
}

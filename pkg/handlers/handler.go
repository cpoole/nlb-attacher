package handlers

import (
	v1 "k8s.io/api/core/v1"
)

// Handler is implemented by any handler.
// The Handle method is used to process event
type Handler interface {
	Init(tgAnnotation string, annotationEnabledValue string) error
	PodCreated(created *v1.Pod)
	PodDeleted(deleted *v1.Pod)
	PodUpdated(oldPod, newPod *v1.Pod)
	TestHandler()
}

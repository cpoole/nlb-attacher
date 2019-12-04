package event

// Event - This struct is passed into the workqueue and must be hashable
type Event struct {
	Key       string
	Reason    string
	EventType string
	Namespace string
}

package aws

import (
	"encoding/json"
	"sync"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

type targetGroupAnnotation struct {
	Arn      string
	PortName string
}

type targetGroupPodAssignment struct {
	tgArn        string
	podIPAddress string
	pod          *v1.Pod
}

var ensureMutex sync.Mutex

// Handler implements handlers.Handler interface
type Handler struct {
	client                   *elbv2.ELBV2
	targetGroups             *[]*elbv2.TargetGroup
	targetGroupAnnotationKey string
	annotationEnableValue    string
}

// Init - initialize the aws nlb modifier
func (handler *Handler) Init(tgAnnotation string, annotationEnabledValue string) error {
	elbClient := elbv2.New(session.New())

	handler.targetGroupAnnotationKey = tgAnnotation
	handler.annotationEnableValue = annotationEnabledValue
	handler.client = elbClient
	defaultTgSlice := make([]*elbv2.TargetGroup, 0)
	handler.targetGroups = &defaultTgSlice

	//TODO: start goroutine to continually update the list of valid load balancers in the background
	//TODO: validate settings and return error
	return nil
}

// PodCreated - Handle the creation event of a pod and ensure it's attached to all of the specified target groups
func (handler *Handler) PodCreated(created *v1.Pod) {
	if created.DeletionTimestamp != nil {
		log.Infof("Discovered pod %s with deletionTimestamp already set. skiping...", created.Name)
		return
	}

	if created.Status.PodIP == "" {
		log.Infof("Recieved event for pod %s without an ip address yet. skipping...", created.Name)
		return
	}

	log.Debugf("created object: %v", created.Name)
	handler.addToTargetGroups(created)
}

// PodDeleted - Handle the deletion event of a pod and ensure it has been removed from all associated target groups
func (handler *Handler) PodDeleted(deleted *v1.Pod) {
	log.Debugf("delete pod: %v", deleted.Name)
	handler.removeFromTargetGroups(deleted)
}

// PodUpdated - Handle pod update
func (handler *Handler) PodUpdated(oldPod, newPod *v1.Pod) {
	log.Debugf("manage update: %v", newPod.Name)
	if newPod.DeletionTimestamp != nil {
		log.Infof("Pod %s is marked for deletion, removing from target group", newPod.Name)
		handler.removeFromTargetGroups(newPod)
	} else {
		log.Infof("Ensuring pod %s is properly attached to the target group", newPod.Name)
		handler.addToTargetGroups(newPod)
	}
}

// TestHandler tests the configurarion by printing dummy lines.
func (handler *Handler) TestHandler() {
	log.Debug("testing")
}

//TODO: re-add this when implementing "target group total control"
//func collectTargetGroups(client *kubernetes.Clientset) map[string][]targetGroupPodInfo {
//	//initial list
//	podTgMapMutex.Lock()
//
//	pods, err := client.CoreV1().Pods("").List(listOptions)
//	if err != nil {
//		log.Fatalln("failed to get pods:", err)
//	}
//
//	//key is the target group arn, value is an array of targetGroupPodInfoStructs
//	targetGroupMap := make(map[string][]targetGroupPodInfo)
//
//	// collect initial annotations
//	for _, pod := range pods.Items {
//		log.Debug(fmt.Sprintf("collectTargetGroups: inspecting pod - %s\n", pod.GetName()))
//		targetGroupMap, _ = addPodToMap(pod, targetGroupMap)
//
//	}
//	podTgMapMutex.Unlock()
//
//	return targetGroupMap
//}

func (handler *Handler) getPodTargetGroupAssignments(pod *v1.Pod) []targetGroupPodAssignment {
	tgAnnotations := make([]targetGroupAnnotation, 0)
	for annotation, value := range pod.GetAnnotations() {
		if annotation == handler.targetGroupAnnotationKey {
			tgAnnotations = serializeAnnotation(value)
		}
	}

	assignments := make([]targetGroupPodAssignment, 0)
	for _, annotation := range tgAnnotations {
		assignments = append(assignments, targetGroupPodAssignment{
			tgArn:        annotation.Arn,
			podIPAddress: pod.Status.PodIP,
			pod:          pod,
		})
	}
	return assignments
}

func serializeAnnotation(value string) []targetGroupAnnotation {
	var targetGroups []targetGroupAnnotation
	err := json.Unmarshal([]byte(value), &targetGroups)
	if err != nil {
		log.Fatalf("Failed to serialize annotations: %s", err)
	}

	return targetGroups
}

func (handler *Handler) addToTargetGroups(pod *v1.Pod) {
	podTargetGroupAssignments := handler.getPodTargetGroupAssignments(pod)

	for _, assignment := range podTargetGroupAssignments {
		ips := []string{assignment.podIPAddress}
		handler.registerTargets(ips, assignment.tgArn)
	}
}

func (handler *Handler) removeFromTargetGroups(pod *v1.Pod) {
	podTargetGroupAssignments := handler.getPodTargetGroupAssignments(pod)

	for _, assignment := range podTargetGroupAssignments {
		handler.deregisterTargets(assignment.podIPAddress, assignment.tgArn)
	}
}

func (handler *Handler) deregisterTargets(ip string, tgArn string) {
	input := &elbv2.DeregisterTargetsInput{
		TargetGroupArn: aws.String(tgArn),
		Targets: []*elbv2.TargetDescription{
			{
				Id: aws.String(ip),
			},
		},
	}

	result, err := handler.client.DeregisterTargets(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elbv2.ErrCodeTargetGroupNotFoundException:
				log.Error(elbv2.ErrCodeTargetGroupNotFoundException, aerr.Error())
			case elbv2.ErrCodeInvalidTargetException:
				log.Error(elbv2.ErrCodeInvalidTargetException, aerr.Error())
			default:
				log.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Error(err.Error())
		}
		return
	}
	log.Infof("Successfully detached: %v from target group %s", ip, tgArn)

	log.Info(result)
}

func (handler *Handler) registerTargets(ips []string, tgArn string) {
	input := &elbv2.RegisterTargetsInput{
		TargetGroupArn: aws.String(tgArn),
		Targets:        make([]*elbv2.TargetDescription, 0),
	}

	for _, ip := range ips {
		local := ip
		input.Targets = append(input.Targets, &elbv2.TargetDescription{Id: &local})
		log.Debugf("Attempting to attach: %s", ip)
	}

	result, err := handler.client.RegisterTargets(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elbv2.ErrCodeTargetGroupNotFoundException:
				log.Error(elbv2.ErrCodeTargetGroupNotFoundException, aerr.Error())
			case elbv2.ErrCodeTooManyTargetsException:
				log.Error(elbv2.ErrCodeTooManyTargetsException, aerr.Error())
			case elbv2.ErrCodeInvalidTargetException:
				log.Error(elbv2.ErrCodeInvalidTargetException, aerr.Error())
			case elbv2.ErrCodeTooManyRegistrationsForTargetIdException:
				log.Error(elbv2.ErrCodeTooManyRegistrationsForTargetIdException, aerr.Error())
			default:
				log.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Error(err.Error())
		}
		return
	}
	log.Infof("Successfully attached: %v to target group %s", ips, tgArn)

	log.Debug(result)
}

func getAllNetworkLoadbalancers(client *elbv2.ELBV2) *[]*elbv2.LoadBalancer {
	//todo: make this paginate and assemble all load balancers

	input := &elbv2.DescribeLoadBalancersInput{}

	result, err := client.DescribeLoadBalancers(input)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elbv2.ErrCodeLoadBalancerNotFoundException:
				log.Error(elbv2.ErrCodeLoadBalancerNotFoundException, aerr.Error())
			default:
				log.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Error(err.Error())
		}

		panic("Failed to describe load balancers")
	}

	return &result.LoadBalancers
}

func getTargetGroups(client *elbv2.ELBV2, groupArns []*string) *[]*elbv2.TargetGroup {
	//todo: make this paginate and assemble all load balancers

	//for _, arn := range groupArns {
	//	log.Debug(*arn)
	//}

	if len(groupArns) == 0 {
		local := make([]*elbv2.TargetGroup, 0)
		return &local
	}

	input := &elbv2.DescribeTargetGroupsInput{
		TargetGroupArns: groupArns,
	}

	result, err := client.DescribeTargetGroups(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elbv2.ErrCodeLoadBalancerNotFoundException:
				log.Error(elbv2.ErrCodeLoadBalancerNotFoundException, aerr.Error())
			case elbv2.ErrCodeTargetGroupNotFoundException:
				log.Error(elbv2.ErrCodeTargetGroupNotFoundException, aerr.Error())
			default:
				log.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Error(err.Error())
		}
		panic("failed to describe target groups")
	}
	return &result.TargetGroups
}

// ensurePodsAreAttached - ensure that the following target groups have the correct IP addresses attached
func (handler *Handler) ensurePodsAreAttached(context *Handler, tgPodMap map[string][]string) {
	//todo: make this paginate and assemble all load balancers

	ensureMutex.Lock()
	for tgArn, podAddresses := range tgPodMap {
		log.Debugf("EnsurePodsAreAttached: key - %s , value - %s", tgArn, podAddresses)
		input := &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: aws.String(tgArn),
		}

		result, err := context.client.DescribeTargetHealth(input)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case elbv2.ErrCodeInvalidTargetException:
					log.Error(elbv2.ErrCodeInvalidTargetException, aerr.Error())
				case elbv2.ErrCodeTargetGroupNotFoundException:
					log.Error(elbv2.ErrCodeTargetGroupNotFoundException, aerr.Error())
				case elbv2.ErrCodeHealthUnavailableException:
					log.Error(elbv2.ErrCodeHealthUnavailableException, aerr.Error())
				default:
					log.Error(aerr.Error())
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				log.Error(err.Error())
			}
		}
		log.Debug(result)

		ipsToRegister := make([]string, 0)
		registeredIps := make([]string, 0)
		for _, target := range result.TargetHealthDescriptions {
			registeredIps = append(registeredIps, *target.Target.Id)
		}
		for _, ip := range podAddresses {
			if !contains(registeredIps, ip) {
				ipsToRegister = append(ipsToRegister, ip)
			}
		}

		handler.registerTargets(ipsToRegister, tgArn)
	}
	ensureMutex.Unlock()
}

func contains(arr []string, target string) bool {
	for _, val := range arr {
		if val == target {
			return true
		}
	}
	return false
}

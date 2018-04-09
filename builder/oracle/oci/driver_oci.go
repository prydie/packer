package oci

import (
	"context"
	"errors"
	"fmt"
	"time"

	core "github.com/oracle/oci-go-sdk/core"
)

// driverOCI implements the Driver interface and communicates with Oracle
// OCI.
type driverOCI struct {
	computeClient core.ComputeClient
	vcnClient     core.VirtualNetworkClient
	cfg           *Config
}

// NewDriverOCI Creates a new driverOCI with a connected compute client and a connected vcn client.
func NewDriverOCI(cfg *Config) (Driver, error) {
	client, err := core.NewComputeClientWithConfigurationProvider(cfg.ConfigProvider)
	if err != nil {
		return nil, err
	}

	vcnclient, err := core.NewVirtualNetworkClientWithConfigurationProvider(cfg.ConfigProvider)
	if err != nil {
		return nil, err
	}

	return &driverOCI{computeClient: client, vcnClient: vcnclient, cfg: cfg}, nil
}

// CreateInstance creates a new compute instance.
func (d *driverOCI) CreateInstance(publicKey string) (string, error) {

	instance, err := d.computeClient.LaunchInstance(context.TODO(), core.LaunchInstanceRequest{LaunchInstanceDetails: core.LaunchInstanceDetails{
		AvailabilityDomain: &d.cfg.AvailabilityDomain,
		CompartmentId:      &d.cfg.CompartmentID,
		ImageId:            &d.cfg.BaseImageID,
		Shape:              &d.cfg.Shape,
		SubnetId:           &d.cfg.SubnetID,
		Metadata: map[string]string{
			"ssh_authorized_keys": publicKey,
		},
	}})

	if err != nil {
		return "", err
	}

	return *instance.Id, nil
}

// CreateImage creates a new custom image.
func (d *driverOCI) CreateImage(id string) (core.Image, error) {
	res, err := d.computeClient.CreateImage(context.TODO(), core.CreateImageRequest{CreateImageDetails: core.CreateImageDetails{
		CompartmentId: &d.cfg.CompartmentID,
		InstanceId:    &id,
		DisplayName:   &d.cfg.ImageName,
	}})

	if err != nil {
		return core.Image{}, err
	}

	return res.Image, nil
}

// DeleteImage deletes a custom image.
func (d *driverOCI) DeleteImage(id string) error {
	_, err := d.computeClient.DeleteImage(context.TODO(), core.DeleteImageRequest{ImageId: &id})

	return err
}

// GetInstanceIP returns the public or private IP corresponding to the given instance id.
func (d *driverOCI) GetInstanceIP(id string) (string, error) {
	vnics, err := d.computeClient.ListVnicAttachments(context.TODO(), core.ListVnicAttachmentsRequest{
		InstanceId:    &id,
		CompartmentId: &d.cfg.CompartmentID,
	})

	if err != nil {
		return "", err
	}

	if len(vnics.Items) < 1 {
		return "", errors.New("instance has zero VNICs")
	}

	vnic, err := d.vcnClient.GetVnic(context.TODO(), core.GetVnicRequest{VnicId: vnics.Items[0].VnicId})

	if err != nil {
		return "", fmt.Errorf("Error getting VNIC details: %s", err)
	}

	if d.cfg.UsePrivateIP {
		return *vnic.PrivateIp, nil
	}

	return *vnic.PublicIp, nil
}

// TerminateInstance terminates a compute instance.
func (d *driverOCI) TerminateInstance(id string) error {

	_, err := d.computeClient.TerminateInstance(context.TODO(), core.TerminateInstanceRequest{
		InstanceId: &id,
	})

	return err
}

// WaitForImageCreation waits for a provisioning custom image to reach the
// "AVAILABLE" state.
func (d *driverOCI) WaitForImageCreation(id string) error {
	return waitForResourceToReachState(
		func(string) (string, error) {
			image, err := d.computeClient.GetImage(context.TODO(), core.GetImageRequest{ImageId: &id})
			if err != nil {
				return "", err
			}
			return string(image.LifecycleState), nil
		},
		id,
		[]string{"PROVISIONING"},
		"AVAILABLE",
		0,    //Unlimited Retries
		5000, //5 second wait between retries
	)
}

// WaitForInstanceState waits for an instance to reach the a given terminal
// state.
func (d *driverOCI) WaitForInstanceState(id string, waitStates []string, terminalState string) error {
	return waitForResourceToReachState(
		func(string) (string, error) {
			instance, err := d.computeClient.GetInstance(context.TODO(), core.GetInstanceRequest{InstanceId: &id})
			if err != nil {
				return "", err
			}
			return string(instance.LifecycleState), nil
		},
		id,
		waitStates,
		terminalState,
		0,    //Unlimited Retries
		5000, //5 second wait between retries
	)
}

// WaitForResourceToReachState checks the response of a request through a polled get and waits until the desired state or until the max retried has been reached.
func waitForResourceToReachState(GetResourceState func(string) (string, error), id string, waitStates []string, terminalState string, maxRetries int, waitDuration int) error {
	for i := 0; maxRetries == 0 || i < maxRetries; i++ {

		state, err := GetResourceState(id)

		if err != nil {
			return err
		}

		if stringSliceContains(waitStates, state) {
			time.Sleep(time.Duration(waitDuration) * time.Millisecond)
			continue
		} else if state == terminalState {
			return nil
		}

		return fmt.Errorf("Unexpected resource state %s, expecting a waiting state %s or terminal state  %s ", state, waitStates, terminalState)
	}

	return fmt.Errorf("Maximum number of retries (%d) exceeded; resource did not reach state %s", maxRetries, terminalState)
}

// stringSliceContains loops through a slice of strings returning a boolean based on whether a given value is contained in the slice.
func stringSliceContains(slice []string, value string) bool {
	for _, elem := range slice {
		if elem == value {
			return true
		}
	}
	return false
}

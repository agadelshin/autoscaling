/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/apimachinery/pkg/runtime"
)

func (r *VirtualMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-vm-neon-tech-v1-virtualmachine,mutating=true,failurePolicy=fail,sideEffects=None,groups=vm.neon.tech,resources=virtualmachines,verbs=create;update,versions=v1,name=mvirtualmachine.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &VirtualMachine{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *VirtualMachine) Default() {
	// Nothing to do.
}

//+kubebuilder:webhook:path=/validate-vm-neon-tech-v1-virtualmachine,mutating=false,failurePolicy=fail,sideEffects=None,groups=vm.neon.tech,resources=virtualmachines,verbs=create;update,versions=v1,name=vvirtualmachine.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &VirtualMachine{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *VirtualMachine) ValidateCreate() (admission.Warnings, error) {
	// validate .spec.guest.cpus.use and .spec.guest.cpus.max
	if r.Spec.Guest.CPUs.Use < r.Spec.Guest.CPUs.Min {
		return nil, fmt.Errorf(".spec.guest.cpus.use (%v) should be greater than or equal to the .spec.guest.cpus.min (%v)",
			r.Spec.Guest.CPUs.Use,
			r.Spec.Guest.CPUs.Min)
	}
	if r.Spec.Guest.CPUs.Use > r.Spec.Guest.CPUs.Max {
		return nil, fmt.Errorf(".spec.guest.cpus.use (%v) should be less than or equal to the .spec.guest.cpus.max (%v)",
			r.Spec.Guest.CPUs.Use,
			r.Spec.Guest.CPUs.Max)
	}

	// validate .spec.guest.memorySlotSize w.r.t. .spec.guest.memoryProvider
	if r.Spec.Guest.MemoryProvider != nil {
		if err := r.Spec.Guest.ValidateForMemoryProvider(*r.Spec.Guest.MemoryProvider); err != nil {
			return nil, fmt.Errorf(".spec.guest: %w", err)
		}
	}

	// validate .spec.guest.memorySlots.use and .spec.guest.memorySlots.max
	if r.Spec.Guest.MemorySlots.Use < r.Spec.Guest.MemorySlots.Min {
		return nil, fmt.Errorf(".spec.guest.memorySlots.use (%d) should be greater than or equal to the .spec.guest.memorySlots.min (%d)",
			r.Spec.Guest.MemorySlots.Use,
			r.Spec.Guest.MemorySlots.Min)
	}
	if r.Spec.Guest.MemorySlots.Use > r.Spec.Guest.MemorySlots.Max {
		return nil, fmt.Errorf(".spec.guest.memorySlots.use (%d) should be less than or equal to the .spec.guest.memorySlots.max (%d)",
			r.Spec.Guest.MemorySlots.Use,
			r.Spec.Guest.MemorySlots.Max)
	}

	// validate .spec.disk names
	reservedDiskNames := []string{
		"virtualmachineimages",
		"rootdisk",
		"runtime",
		"swapdisk",
		"sysfscgroup",
		"containerdsock",
		"ssh-privatekey",
		"ssh-publickey",
		"ssh-authorized-keys",
	}
	for _, disk := range r.Spec.Disks {
		if slices.Contains(reservedDiskNames, disk.Name) {
			return nil, fmt.Errorf("'%s' is reserved for .spec.disks[].name", disk.Name)
		}
		if len(disk.Name) > 32 {
			return nil, fmt.Errorf("disk name '%s' too long, should be less than or equal to 32", disk.Name)
		}
	}

	// validate .spec.guest.ports[].name
	for _, port := range r.Spec.Guest.Ports {
		if len(port.Name) != 0 && port.Name == "qmp" {
			return nil, errors.New("'qmp' is reserved name for .spec.guest.ports[].name")
		}
	}

	// validate that at most one type of swap is provided:
	if settings := r.Spec.Guest.Settings; settings != nil {
		if settings.Swap != nil && settings.SwapInfo != nil {
			return nil, errors.New("cannot have both 'swap' and 'swapInfo' enabled")
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *VirtualMachine) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	// process immutable fields
	before, _ := old.(*VirtualMachine)

	immutableFields := []struct {
		fieldName string
		getter    func(*VirtualMachine) any
	}{
		{".spec.guest.cpus.min", func(v *VirtualMachine) any { return v.Spec.Guest.CPUs.Min }},
		{".spec.guest.cpus.max", func(v *VirtualMachine) any { return v.Spec.Guest.CPUs.Max }},
		{".spec.guest.memorySlots.min", func(v *VirtualMachine) any { return v.Spec.Guest.MemorySlots.Min }},
		{".spec.guest.memorySlots.max", func(v *VirtualMachine) any { return v.Spec.Guest.MemorySlots.Max }},
		// nb: we don't check memoryProvider here, so that it's allowed to be mutable as a way of
		// getting flexibility to solidify the memory provider or change it across restarts.
		// ref https://github.com/neondatabase/autoscaling/pull/970#discussion_r1644225986
		{".spec.guest.memoryProvider", func(v *VirtualMachine) any { return v.Spec.Guest.MemoryProvider }},
		{".spec.guest.ports", func(v *VirtualMachine) any { return v.Spec.Guest.Ports }},
		{".spec.guest.rootDisk", func(v *VirtualMachine) any { return v.Spec.Guest.RootDisk }},
		{".spec.guest.command", func(v *VirtualMachine) any { return v.Spec.Guest.Command }},
		{".spec.guest.args", func(v *VirtualMachine) any { return v.Spec.Guest.Args }},
		{".spec.guest.env", func(v *VirtualMachine) any { return v.Spec.Guest.Env }},
		{".spec.guest.settings", func(v *VirtualMachine) any {
			if v.Spec.Guest.Settings == nil {
				//nolint:gocritic // linter complains that we could say 'nil' directly. It's typed vs untyped nil.
				return v.Spec.Guest.Settings
			} else {
				// Selectively allow swap fields to change between Swap and SwapInfo. More below.
				return v.Spec.Guest.Settings.WithoutSwapFields()
			}
		}},
		{".spec.disks", func(v *VirtualMachine) any { return v.Spec.Disks }},
		{".spec.podResources", func(v *VirtualMachine) any { return v.Spec.PodResources }},
		{".spec.enableAcceleration", func(v *VirtualMachine) any { return v.Spec.EnableAcceleration }},
		{".spec.enableSSH", func(v *VirtualMachine) any { return v.Spec.EnableSSH }},
		{".spec.initScript", func(v *VirtualMachine) any { return v.Spec.InitScript }},
	}

	for _, info := range immutableFields {
		if !reflect.DeepEqual(info.getter(r), info.getter(before)) {
			return nil, fmt.Errorf("%s is immutable", info.fieldName)
		}
	}

	// validate swap changes by comparing the SwapInfo for each.
	//
	// If there's an error with the old object, but NOT an error with the new one, we'll allow the
	// new one to proceed. This is to prevent any VirtualMachine objects getting stuck during
	// rollout of swap changes, in case there's logic bugs in handling the change.
	//
	// If we didn't have that exception, we could *in theory* end up with an object in a bad state,
	// but be unable to fix it because the old state is bad - even if the new one is ok - because
	// the webhook would return an error from the old state being invalid, which disallows the update
	if r.Spec.Guest.Settings != nil /* from above, if new GuestSettings != nil, then old is as well */ {
		newSwapInfo, err := r.Spec.Guest.Settings.GetSwapInfo()
		if err != nil {
			return nil, err
		}
		oldSwapInfo, err := before.Spec.Guest.Settings.GetSwapInfo()
		if err != nil {
			// do nothing; we'll allow fixing broken objects.
		} else {
			if !reflect.DeepEqual(newSwapInfo, oldSwapInfo) {
				return nil, errors.New(".spec.guest.settings.{swap,swapInfo} is immutable")
			}
		}
	}

	// validate .spec.guest.cpu.use
	if r.Spec.Guest.CPUs.Use < r.Spec.Guest.CPUs.Min {
		return nil, fmt.Errorf(".cpus.use (%v) should be greater than or equal to the .cpus.min (%v)",
			r.Spec.Guest.CPUs.Use,
			r.Spec.Guest.CPUs.Min)
	}
	if r.Spec.Guest.CPUs.Use > r.Spec.Guest.CPUs.Max {
		return nil, fmt.Errorf(".cpus.use (%v) should be less than or equal to the .cpus.max (%v)",
			r.Spec.Guest.CPUs.Use,
			r.Spec.Guest.CPUs.Max)
	}

	// validate .spec.guest.memorySlots.use
	if r.Spec.Guest.MemorySlots.Use < r.Spec.Guest.MemorySlots.Min {
		return nil, fmt.Errorf(".memorySlots.use (%d) should be greater than or equal to the .memorySlots.min (%d)",
			r.Spec.Guest.MemorySlots.Use,
			r.Spec.Guest.MemorySlots.Min)
	}
	if r.Spec.Guest.MemorySlots.Use > r.Spec.Guest.MemorySlots.Max {
		return nil, fmt.Errorf(".memorySlots.use (%d) should be less than or equal to the .memorySlots.max (%d)",
			r.Spec.Guest.MemorySlots.Use,
			r.Spec.Guest.MemorySlots.Max)
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *VirtualMachine) ValidateDelete() (admission.Warnings, error) {
	// No deletion validation required currently.
	return nil, nil
}

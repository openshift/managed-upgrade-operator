// +build !ignore_autogenerated

// Code generated by operator-sdk. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in Conditions) DeepCopyInto(out *Conditions) {
	{
		in := &in
		*out = make(Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
		return
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Conditions.
func (in Conditions) DeepCopy() Conditions {
	if in == nil {
		return nil
	}
	out := new(Conditions)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Drain) DeepCopyInto(out *Drain) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Drain.
func (in *Drain) DeepCopy() *Drain {
	if in == nil {
		return nil
	}
	out := new(Drain)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HealthCheck) DeepCopyInto(out *HealthCheck) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HealthCheck.
func (in *HealthCheck) DeepCopy() *HealthCheck {
	if in == nil {
		return nil
	}
	out := new(HealthCheck)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Notification) DeepCopyInto(out *Notification) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Notification.
func (in *Notification) DeepCopy() *Notification {
	if in == nil {
		return nil
	}
	out := new(Notification)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Scaling) DeepCopyInto(out *Scaling) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Scaling.
func (in *Scaling) DeepCopy() *Scaling {
	if in == nil {
		return nil
	}
	out := new(Scaling)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SubscriptionUpdate) DeepCopyInto(out *SubscriptionUpdate) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SubscriptionUpdate.
func (in *SubscriptionUpdate) DeepCopy() *SubscriptionUpdate {
	if in == nil {
		return nil
	}
	out := new(SubscriptionUpdate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Update) DeepCopyInto(out *Update) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Update.
func (in *Update) DeepCopy() *Update {
	if in == nil {
		return nil
	}
	out := new(Update)
	in.DeepCopyInto(out)
	return out
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeCondition.
func (in *UpgradeCondition) DeepCopy() *UpgradeCondition {
	if in == nil {
		return nil
	}
	out := new(UpgradeCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpgradeConfig) DeepCopyInto(out *UpgradeConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeConfig.
func (in *UpgradeConfig) DeepCopy() *UpgradeConfig {
	if in == nil {
		return nil
	}
	out := new(UpgradeConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *UpgradeConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpgradeConfigList) DeepCopyInto(out *UpgradeConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]UpgradeConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeConfigList.
func (in *UpgradeConfigList) DeepCopy() *UpgradeConfigList {
	if in == nil {
		return nil
	}
	out := new(UpgradeConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *UpgradeConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpgradeConfigSpec) DeepCopyInto(out *UpgradeConfigSpec) {
	*out = *in
	out.Desired = in.Desired
	if in.SubscriptionUpdates != nil {
		in, out := &in.SubscriptionUpdates, &out.SubscriptionUpdates
		*out = make([]SubscriptionUpdate, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeConfigSpec.
func (in *UpgradeConfigSpec) DeepCopy() *UpgradeConfigSpec {
	if in == nil {
		return nil
	}
	out := new(UpgradeConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpgradeConfigStatus) DeepCopyInto(out *UpgradeConfigStatus) {
	*out = *in
	if in.History != nil {
		in, out := &in.History, &out.History
		*out = make(UpgradeHistories, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.NotificationEvent = in.NotificationEvent
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeConfigStatus.
func (in *UpgradeConfigStatus) DeepCopy() *UpgradeConfigStatus {
	if in == nil {
		return nil
	}
	out := new(UpgradeConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in UpgradeHistories) DeepCopyInto(out *UpgradeHistories) {
	{
		in := &in
		*out = make(UpgradeHistories, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
		return
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeHistories.
func (in UpgradeHistories) DeepCopy() UpgradeHistories {
	if in == nil {
		return nil
	}
	out := new(UpgradeHistories)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpgradeHistory) DeepCopyInto(out *UpgradeHistory) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.StartTime != nil {
		in, out := &in.StartTime, &out.StartTime
		*out = (*in).DeepCopy()
	}
	if in.CompleteTime != nil {
		in, out := &in.CompleteTime, &out.CompleteTime
		*out = (*in).DeepCopy()
	}
	if in.WorkerStartTime != nil {
		in, out := &in.WorkerStartTime, &out.WorkerStartTime
		*out = (*in).DeepCopy()
	}
	if in.WorkerCompleteTime != nil {
		in, out := &in.WorkerCompleteTime, &out.WorkerCompleteTime
		*out = (*in).DeepCopy()
	}
	out.HealthCheck = in.HealthCheck
	out.Scaling = in.Scaling
	out.NodeDrain = in.NodeDrain
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpgradeHistory.
func (in *UpgradeHistory) DeepCopy() *UpgradeHistory {
	if in == nil {
		return nil
	}
	out := new(UpgradeHistory)
	in.DeepCopyInto(out)
	return out
}

/*
Copyright The Kmodules Authors.

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

package v1beta1

import (
	"context"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	policy "k8s.io/api/policy/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	kutil "kmodules.xyz/client-go"
)

func CreateOrPatchPodSecurityPolicy(ctx context.Context, c kubernetes.Interface, meta metav1.ObjectMeta, transform func(*policy.PodSecurityPolicy) *policy.PodSecurityPolicy) (*policy.PodSecurityPolicy, kutil.VerbType, error) {
	cur, err := c.PolicyV1beta1().PodSecurityPolicies().Get(ctx, meta.Name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		glog.V(3).Infof("Creating PodSecurityPolicy %s.", meta.Name)
		out, err := c.PolicyV1beta1().PodSecurityPolicies().Create(ctx, transform(&policy.PodSecurityPolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       "PodSecurityPolicy",
				APIVersion: policy.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta,
		}), metav1.CreateOptions{})
		return out, kutil.VerbCreated, err
	} else if err != nil {
		return nil, kutil.VerbUnchanged, err
	}
	return PatchPodSecurityPolicy(ctx, c, cur, transform)
}

func PatchPodSecurityPolicy(ctx context.Context, c kubernetes.Interface, cur *policy.PodSecurityPolicy, transform func(*policy.PodSecurityPolicy) *policy.PodSecurityPolicy) (*policy.PodSecurityPolicy, kutil.VerbType, error) {
	return PatchPodSecurityPolicyObject(ctx, c, cur, transform(cur.DeepCopy()))
}

func PatchPodSecurityPolicyObject(ctx context.Context, c kubernetes.Interface, cur, mod *policy.PodSecurityPolicy) (*policy.PodSecurityPolicy, kutil.VerbType, error) {
	curJson, err := json.Marshal(cur)
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}

	modJson, err := json.Marshal(mod)
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(curJson, modJson, policy.PodSecurityPolicy{})
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}
	if len(patch) == 0 || string(patch) == "{}" {
		return cur, kutil.VerbUnchanged, nil
	}
	glog.V(3).Infof("Patching PodSecurityPolicy %s with %s.", cur.Name, string(patch))
	out, err := c.PolicyV1beta1().PodSecurityPolicies().Patch(ctx, cur.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return out, kutil.VerbPatched, err
}

func TryUpdatePodSecurityPolicy(ctx context.Context, c kubernetes.Interface, meta metav1.ObjectMeta, transform func(*policy.PodSecurityPolicy) *policy.PodSecurityPolicy) (result *policy.PodSecurityPolicy, err error) {
	attempt := 0
	err = wait.PollImmediate(kutil.RetryInterval, kutil.RetryTimeout, func() (bool, error) {
		attempt++
		cur, e2 := c.PolicyV1beta1().PodSecurityPolicies().Get(ctx, meta.Name, metav1.GetOptions{})
		if kerr.IsNotFound(e2) {
			return false, e2
		} else if e2 == nil {
			result, e2 = c.PolicyV1beta1().PodSecurityPolicies().Update(ctx, transform(cur.DeepCopy()), metav1.UpdateOptions{})
			return e2 == nil, nil
		}
		glog.Errorf("Attempt %d failed to update PodSecurityPolicy %s due to %v.", attempt, cur.Name, e2)
		return false, nil
	})

	if err != nil {
		err = errors.Errorf("failed to update PodSecurityPolicy %s after %d attempts due to %v", meta.Name, attempt, err)
	}
	return
}

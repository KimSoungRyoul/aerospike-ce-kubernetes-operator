package controller

import (
	"context"
	"fmt"
	"reflect"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

const (
	podServiceReaderRole = "acko-pod-service-reader"
)

// reconcilePodServiceRBAC creates or cleans up the Role and RoleBinding that
// allow the Aerospike pod's service account to read its own per-pod Service.
// This is required for the init container to resolve LoadBalancer/NodePort
// external addresses via the Kubernetes API.
func (r *AerospikeClusterReconciler) reconcilePodServiceRBAC(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	needsRBAC := cluster.Spec.PodService != nil &&
		cluster.Spec.PodService.ServiceType != "" &&
		cluster.Spec.PodService.ServiceType != "ClusterIP"

	roleName := fmt.Sprintf("%s-%s", cluster.Name, podServiceReaderRole)
	if !needsRBAC {
		return r.cleanupPodServiceRBAC(ctx, cluster, roleName)
	}

	log := logf.FromContext(ctx)
	saName := "default"

	desiredRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"services"},
			Verbs:     []string{"get"},
		},
	}

	// Reconcile Role
	existingRole := &rbacv1.Role{}
	err := r.Get(ctx, types.NamespacedName{Name: roleName, Namespace: cluster.Namespace}, existingRole)
	if errors.IsNotFound(err) {
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: cluster.Namespace,
			},
			Rules: desiredRules,
		}
		if err := r.setOwnerRef(cluster, role); err != nil {
			return err
		}
		log.Info("Creating pod service reader Role", "name", roleName)
		if err := r.Create(ctx, role); err != nil {
			return fmt.Errorf("creating pod service reader role %s: %w", roleName, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting pod service reader role %s: %w", roleName, err)
	} else if !reflect.DeepEqual(existingRole.Rules, desiredRules) {
		// Role exists but has drifted from the desired spec; correct it.
		existingRole.Rules = desiredRules
		if err := r.setOwnerRef(cluster, existingRole); err != nil {
			return err
		}
		log.Info("Updating drifted pod service reader Role", "name", roleName)
		if err := r.Update(ctx, existingRole); err != nil {
			return fmt.Errorf("updating pod service reader role %s: %w", roleName, err)
		}
	}

	// Reconcile RoleBinding
	bindingName := roleName
	desiredRoleRef := rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     roleName,
	}
	desiredSubjects := []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: cluster.Namespace,
		},
	}

	existingBinding := &rbacv1.RoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: bindingName, Namespace: cluster.Namespace}, existingBinding)
	if errors.IsNotFound(err) {
		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: cluster.Namespace,
			},
			RoleRef:  desiredRoleRef,
			Subjects: desiredSubjects,
		}
		if err := r.setOwnerRef(cluster, binding); err != nil {
			return err
		}
		log.Info("Creating pod service reader RoleBinding", "name", bindingName)
		if err := r.Create(ctx, binding); err != nil {
			return fmt.Errorf("creating pod service reader rolebinding %s: %w", bindingName, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting pod service reader rolebinding %s: %w", bindingName, err)
	} else if !reflect.DeepEqual(existingBinding.RoleRef, desiredRoleRef) ||
		!reflect.DeepEqual(existingBinding.Subjects, desiredSubjects) {
		// RoleBinding exists but has drifted from the desired spec.
		// RoleRef is immutable, so a divergence there requires recreation;
		// Subjects can be updated in place.
		if !reflect.DeepEqual(existingBinding.RoleRef, desiredRoleRef) {
			log.Info("Recreating pod service reader RoleBinding due to immutable RoleRef drift", "name", bindingName)
			if err := r.Delete(ctx, existingBinding); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("deleting drifted pod service reader rolebinding %s: %w", bindingName, err)
			}
			binding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: cluster.Namespace,
				},
				RoleRef:  desiredRoleRef,
				Subjects: desiredSubjects,
			}
			if err := r.setOwnerRef(cluster, binding); err != nil {
				return err
			}
			if err := r.Create(ctx, binding); err != nil {
				return fmt.Errorf("recreating pod service reader rolebinding %s: %w", bindingName, err)
			}
		} else {
			existingBinding.Subjects = desiredSubjects
			if err := r.setOwnerRef(cluster, existingBinding); err != nil {
				return err
			}
			log.Info("Updating drifted pod service reader RoleBinding", "name", bindingName)
			if err := r.Update(ctx, existingBinding); err != nil {
				return fmt.Errorf("updating pod service reader rolebinding %s: %w", bindingName, err)
			}
		}
	}

	return nil
}

// cleanupPodServiceRBAC removes the Role and RoleBinding when no longer needed.
func (r *AerospikeClusterReconciler) cleanupPodServiceRBAC(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	roleName string,
) error {
	log := logf.FromContext(ctx)

	binding := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, types.NamespacedName{Name: roleName, Namespace: cluster.Namespace}, binding); err == nil {
		log.Info("Deleting pod service reader RoleBinding", "name", roleName)
		if err := r.Delete(ctx, binding); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting pod service reader rolebinding %s: %w", roleName, err)
		}
	}

	role := &rbacv1.Role{}
	if err := r.Get(ctx, types.NamespacedName{Name: roleName, Namespace: cluster.Namespace}, role); err == nil {
		log.Info("Deleting pod service reader Role", "name", roleName)
		if err := r.Delete(ctx, role); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting pod service reader role %s: %w", roleName, err)
		}
	}

	return nil
}

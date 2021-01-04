/*


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

package controllers

import (
	mygroupv1 "K8S-CRD-Demo/api/v1"
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// create a new deploy object
func NewDeploy(owner *mygroupv1.Mykind, logger logr.Logger, scheme *runtime.Scheme) *appsv1.Deployment {
	labels := map[string]string{"app": owner.Name}
	selector := &metav1.LabelSelector{MatchLabels: labels}
	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name,
			Namespace: owner.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: owner.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            owner.Name,
							Image:           owner.Spec.Image,
							Ports:           []corev1.ContainerPort{{ContainerPort: owner.Spec.Port}},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             owner.Spec.Envs,
						},
					},
				},
			},
			Selector: selector,
		},
	}
	// add ControllerReference for deployment
	if err := controllerutil.SetControllerReference(owner, deploy, scheme); err != nil {
		msg := fmt.Sprintf("***SetControllerReference for Deployment %s/%s failed!***", owner.Namespace, owner.Name)
		logger.Error(err, msg)
	}
	return deploy
}

// create a new service object
func NewService(owner *mygroupv1.Mykind, logger logr.Logger, scheme *runtime.Scheme) *corev1.Service {
	srv := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name,
			Namespace: owner.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: owner.Spec.Port, NodePort: owner.Spec.Nodeport}},
			Selector: map[string]string{
				"app": owner.Name,
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}
	// add ControllerReference for service
	if err := controllerutil.SetControllerReference(owner, srv, scheme); err != nil {
		msg := fmt.Sprintf("***setcontrollerReference for Service %s/%s failed!***", owner.Namespace, owner.Name)
		logger.Error(err, msg)
	}
	return srv
}

// MykindReconciler reconciles a Mykind object
type MykindReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mygroup.ips.com.cn,resources=mykinds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mygroup.ips.com.cn,resources=mykinds/status,verbs=get;update;patch
func (r *MykindReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	fmt.Println("---start Reconcile---")
	ctx := context.Background()
	lgr := r.Log.WithValues("mykind", req.NamespacedName)

	// your logic here
	/*1. create/update deploy
	  ========================*/
	mycrd_instance := &mygroupv1.Mykind{}
	if err := r.Get(ctx, req.NamespacedName, mycrd_instance); err != nil {
		lgr.Error(err, "***Get crd instance failed(maybe be deleted)! please check!***")
		return reconcile.Result{}, err
	}
	/*if mycrd_instance.DeletionTimestamp != nil {
		lgr.Info("---Deleting crd instance,cleanup subresources---")
		return reconcile.Result{}, nil
	}*/
	oldDeploy := &appsv1.Deployment{}
	newDeploy := NewDeploy(mycrd_instance, lgr, r.Scheme)
	if err := r.Get(ctx, req.NamespacedName, oldDeploy); err != nil && errors.IsNotFound(err) {
		lgr.Info("---Creating deploy---")
		// 1. create Deploy
		if err := r.Create(ctx, newDeploy); err != nil {
			lgr.Error(err, "***create deploy failed!***")
			return reconcile.Result{}, err
		}
		lgr.Info("---Create deploy done---")
	} else {
		if !reflect.DeepEqual(oldDeploy.Spec, newDeploy.Spec) {
			lgr.Info("---Updating deploy---")
			oldDeploy.Spec = newDeploy.Spec
			if err := r.Update(ctx, oldDeploy); err != nil {
				lgr.Error(err, "***Update old deploy failed!***")
				return reconcile.Result{}, err
			}
			lgr.Info("---Update deploy done---")
		}
	}
	/*2. create/update Service
	  =========================*/
	oldService := &corev1.Service{}
	newService := NewService(mycrd_instance, lgr, r.Scheme)
	if err := r.Get(ctx, req.NamespacedName, oldService); err != nil && errors.IsNotFound(err) {
		lgr.Info("---Creating service---")
		if err := r.Create(ctx, newService); err != nil {
			lgr.Error(err, "***Create service failed!***")
			return reconcile.Result{}, err
		}
		lgr.Info("---Create service done---")
		return reconcile.Result{}, nil
	} else {
		if !reflect.DeepEqual(oldService.Spec, newService.Spec) {
			lgr.Info("---Updating service---")
			clstip := oldService.Spec.ClusterIP //!!!clusterip unable be changed!!!
			oldService.Spec = newService.Spec
			oldService.Spec.ClusterIP = clstip
			if err := r.Update(ctx, oldService); err != nil {
				lgr.Error(err, "***Update service failed!***")
				return reconcile.Result{}, err
			}
			lgr.Info("---Update service done---")
			return reconcile.Result{}, nil
		}
	}
	lgr.Info("!!!err from Get maybe is nil,please check!!!")
	//end your logic
	return ctrl.Result{}, nil
}

func (r *MykindReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mygroupv1.Mykind{}).
		Complete(r)
}

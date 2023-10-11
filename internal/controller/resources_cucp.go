/*
Copyright 2023 The Nephio Authors.

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
package controller

import (
	"encoding/json"
	"strconv"

	"github.com/go-logr/logr"
	nephiov1alpha1 "github.com/nephio-project/api/nf_deployments/v1alpha1"
	configref "github.com/nephio-project/api/references/v1alpha1"
	free5gccontrollers "github.com/nephio-project/free5gc/controllers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CuCpResources struct {
}

func (resource CuCpResources) GetServiceAccount() []*corev1.ServiceAccount {

	serviceAccount1 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "oai-gnb-cu-cp-sa",
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
	}

	return []*corev1.ServiceAccount{serviceAccount1}
}

func (resource CuCpResources) GetConfigMap(log logr.Logger, ranDeployment *nephiov1alpha1.NFDeployment, configInstancesMap map[string][]*configref.Config) []*corev1.ConfigMap {

	n2Ip, err := free5gccontrollers.GetFirstInterfaceConfigIPv4(ranDeployment.Spec.Interfaces, "n2")
	if err != nil {
		log.Error(err, "Interface n2 not found in RANDeployment Spec")
		return nil
	}

	quotedN2Ip := strconv.Quote(n2Ip)

	e1Ip, err := free5gccontrollers.GetFirstInterfaceConfigIPv4(ranDeployment.Spec.Interfaces, "e1")
	if err != nil {
		log.Error(err, "Interface e1 not found in RANDeployment Spec")
		return nil
	}

	quotedE1Ip := strconv.Quote(e1Ip)

	f1cIp, err := free5gccontrollers.GetFirstInterfaceConfigIPv4(ranDeployment.Spec.Interfaces, "f1c")
	if err != nil {
		log.Error(err, "Interface f1c not found in RANDeployment Spec")
		return nil
	}

	quotedF1CIp := strconv.Quote(f1cIp)

	amfDeployment := getConfigInstanceByProvider(log, configInstancesMap["NFDeployment"], "oai-amf.nephio.org")

	amfIp, err := free5gccontrollers.GetFirstInterfaceConfigIPv4(amfDeployment.Spec.Interfaces, "n2")
	if err != nil {
		log.Error(err, "AMF IP not found in Config Refs AMFDeployment")
		return nil
	}

	quotedAmfIp := strconv.Quote(amfIp)

	params3gpp := &Params3gppCrd{}
	if err := json.Unmarshal(configInstancesMap["Params3gpp"][0].Spec.Config.Raw, params3gpp); err != nil {
		log.Error(err, "Cannot Unmarshal Params3gpp")
		return nil
	}

	templateValues := configurationTemplateValuesForCuCp{
		E1_IP:           quotedE1Ip,
		F1C_IP:          quotedF1CIp,
		N2_IP:           quotedN2Ip,
		AMF_IP:          quotedAmfIp,
		TAC:             params3gpp.Spec.Tac,
		CELL_ID:         params3gpp.Spec.CellIdentity,
		PHY_CELL_ID:     strconv.Itoa(params3gpp.Spec.PhysicalCellId),
		PLMN_MCC:        params3gpp.Spec.Plmn.Mcc,
		PLMN_MNC:        params3gpp.Spec.Plmn.Mnc,
		PLMN_MNC_LENGTH: strconv.Itoa(params3gpp.Spec.Plmn.MncLength),
		NSSAI_SST:       params3gpp.Spec.NssaiList[0].Sst,
		NSSAI_SD:        params3gpp.Spec.NssaiList[0].Sd,
	}

	configuration, err := renderConfigurationTemplateForCuCp(templateValues)
	if err != nil {
		log.Error(err, "Could not render CU CP configuration template.")
		return nil
	}

	println("created one: ", configuration)

	configMap1 := &corev1.ConfigMap{
		Data: map[string]string{
			"mounted.conf": configuration,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "oai-gnb-cu-cp-configmap",
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
	}

	return []*corev1.ConfigMap{configMap1}
}

func (resource CuCpResources) createNetworkAttachmentDefinitionNetworks(templateName string, ranDeploymentSpec *nephiov1alpha1.NFDeploymentSpec) (string, error) {
	return free5gccontrollers.CreateNetworkAttachmentDefinitionNetworks(templateName, map[string][]nephiov1alpha1.InterfaceConfig{
		"e1":  free5gccontrollers.GetInterfaceConfigs(ranDeploymentSpec.Interfaces, "e1"),
		"n2":  free5gccontrollers.GetInterfaceConfigs(ranDeploymentSpec.Interfaces, "n2"),
		"f1c": free5gccontrollers.GetInterfaceConfigs(ranDeploymentSpec.Interfaces, "f1c"),
	})
}

func (resource CuCpResources) GetDeployment(ranDeployment *nephiov1alpha1.NFDeployment) []*appsv1.Deployment {

	spec := ranDeployment.Spec

	networkAttachmentDefinitionNetworks, err := resource.createNetworkAttachmentDefinitionNetworks("oai-ran-cu-cp", &spec)

	if err != nil {
		return nil
	}

	podAnnotations := make(map[string]string)
	podAnnotations[free5gccontrollers.NetworksAnnotation] = networkAttachmentDefinitionNetworks

	deployment1 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/name": "oai-gnb-cu-cp",
			},
			Name: "oai-gnb-cu-cp",
		},
		Spec: appsv1.DeploymentSpec{
			Paused:   false,
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "oai-gnb-cu-cp",
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.DeploymentStrategyType("Recreate"),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                    "oai-gnb-cu-cp-cp",
						"app.kubernetes.io/name": "oai-gnb-cu-cp",
					},
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{

						corev1.Container{
							SecurityContext: &corev1.SecurityContext{
								Privileged: boolPtr(true),
							},
							Stdin:     false,
							StdinOnce: false,
							TTY:       false,
							VolumeMounts: []corev1.VolumeMount{

								corev1.VolumeMount{
									Name:      "configuration",
									ReadOnly:  false,
									SubPath:   "mounted.conf",
									MountPath: "/opt/oai-gnb/etc/mounted.conf",
								},
							},
							Env: []corev1.EnvVar{

								corev1.EnvVar{
									Name:  "TZ",
									Value: "Europe/Paris",
								},
								corev1.EnvVar{
									Name:  "USE_ADDITIONAL_OPTIONS",
									Value: "--sa --log_config.global_log_options level,nocolor,time",
								},
								corev1.EnvVar{
									Name:  "USE_VOLUMED_CONF",
									Value: "yes",
								},
							},
							Image: "docker.io/oaisoftwarealliance/oai-gnb:2023.w19",
							Ports: []corev1.ContainerPort{

								corev1.ContainerPort{
									Name:          "n2",
									Protocol:      corev1.Protocol("SCTP"),
									ContainerPort: 36412,
								},
								corev1.ContainerPort{
									ContainerPort: 38462,
									Name:          "e1",
									Protocol:      corev1.Protocol("SCTP"),
								},
								corev1.ContainerPort{
									ContainerPort: 38472,
									Name:          "f1c",
									Protocol:      corev1.Protocol("UDP"),
								},
							},
							Name: "gnbcucp",
						},
					},
					DNSPolicy:   corev1.DNSPolicy("ClusterFirst"),
					HostNetwork: false,
					HostPID:     false,
					Volumes: []corev1.Volume{

						corev1.Volume{
							Name: "configuration",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "oai-gnb-cu-cp-configmap",
									},
								},
							},
						},
					},
					TerminationGracePeriodSeconds: int64Ptr(5),
					HostIPC:                       false,
					RestartPolicy:                 corev1.RestartPolicy("Always"),
					SchedulerName:                 "default-scheduler",
					ServiceAccountName:            "oai-gnb-cu-cp-sa",
				},
			},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
	}

	return []*appsv1.Deployment{deployment1}
}

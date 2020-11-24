/*
This file is part of Cloud Native PostgreSQL.

Copyright (C) 2019-2020 2ndQuadrant Italia SRL. Exclusively licensed to 2ndQuadrant Limited.
*/

package specs

import (
	"fmt"

	"github.com/kballard/go-shellquote"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.2ndquadrant.com/k8s/cloud-native-postgresql/api/v1alpha1"
	"gitlab.2ndquadrant.com/k8s/cloud-native-postgresql/pkg/versions"
)

// CreatePrimaryJobViaInitdb create a new primary instance in a Pod
func CreatePrimaryJobViaInitdb(cluster v1alpha1.Cluster, nodeSerial int32) *batchv1.Job {
	podName := fmt.Sprintf("%s-%v", cluster.Name, nodeSerial)
	jobName := fmt.Sprintf("%s-%v-initdb", cluster.Name, nodeSerial)

	initCommand := []string{
		"/controller/manager",
		"instance",
		"init",
		"-pw-file", "/etc/superuser-secret/password",
		"-parent-node", cluster.GetServiceReadWriteName(),
	}

	if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.InitDB != nil {
		initCommand = append(
			initCommand,
			"--initdb-flags",
			shellquote.Join(cluster.Spec.Bootstrap.InitDB.Options...))
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "pgdata",
			MountPath: "/var/lib/postgresql/data",
		},
		{
			Name:      "config",
			MountPath: "/etc/configuration",
		},
		{
			Name:      "superuser-secret",
			MountPath: "/etc/superuser-secret",
		},
		{
			Name:      "controller",
			MountPath: "/controller",
		},
		{
			Name:      "socket",
			MountPath: "/var/run/postgresql",
		},
	}

	if cluster.ShouldCreateApplicationDatabase() {
		initCommand = append(initCommand,
			"-app-db-name", cluster.Spec.Bootstrap.InitDB.Database,
			"-app-user", cluster.Spec.Bootstrap.InitDB.Owner,
			"-app-pw-file", "/etc/app-secret/password")

		volumeMounts = append(volumeMounts,
			corev1.VolumeMount{
				Name:      "app-secret",
				MountPath: "/etc/app-secret",
			},
		)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "bootstrap-controller",
							Image: versions.GetDefaultOperatorImageName(),
							Command: []string{
								"/manager",
								"bootstrap",
								"/controller/manager",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "controller",
									MountPath: "/controller",
								},
								{
									Name:      "socket",
									MountPath: "/var/run/postgresql",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "PGHOST",
									Value: "/var/run/postgresql",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "bootstrap-instance",
							Image: cluster.GetImageName(),
							Env: []corev1.EnvVar{
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
								{
									Name:  "POD_NAME",
									Value: podName,
								},
								{
									Name:  "CLUSTER_NAME",
									Value: cluster.Name,
								},
								{
									Name:  "NAMESPACE",
									Value: cluster.Namespace,
								},
								{
									Name:  "PGHOST",
									Value: "/var/run/postgresql",
								},
							},
							Command:      initCommand,
							VolumeMounts: volumeMounts,
						},
					},
					Volumes:            createPostgresVolumes(cluster, podName),
					SecurityContext:    CreatePostgresSecurityContext(cluster.GetPostgresUID(), cluster.GetPostgresGID()),
					Affinity:           CreateAffinitySection(cluster.Name, cluster.Spec.Affinity),
					ServiceAccountName: cluster.Name,
					RestartPolicy:      corev1.RestartPolicyNever,
				},
			},
		},
	}

	return job
}

// CreatePrimaryJobViaFullRecovery create a new primary instance in a Pod
func CreatePrimaryJobViaFullRecovery(cluster v1alpha1.Cluster, nodeSerial int32, backup *v1alpha1.Backup) *batchv1.Job {
	podName := fmt.Sprintf("%s-%v", cluster.Name, nodeSerial)
	jobName := fmt.Sprintf("%s-%v-full-recovery", cluster.Name, nodeSerial)

	initCommand := []string{
		"/controller/manager",
		"instance",
		"restore",
		"-pw-file", "/etc/superuser-secret/password",
		"-parent-node", cluster.GetServiceReadWriteName(),
		"-backup-name", cluster.Spec.Bootstrap.FullRecovery.Backup.Name,
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Hostname:  jobName,
					Subdomain: cluster.GetServiceAnyName(),
					InitContainers: []corev1.Container{
						{
							Name:  "bootstrap-controller",
							Image: versions.GetDefaultOperatorImageName(),
							Command: []string{
								"/manager",
								"bootstrap",
								"/controller/manager",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "controller",
									MountPath: "/controller",
								},
								{
									Name:      "socket",
									MountPath: "/var/run/postgresql",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "PGHOST",
									Value: "/var/run/postgresql",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "bootstrap-full-recovery",
							Image: cluster.GetImageName(),
							Env: []corev1.EnvVar{
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
								{
									Name:  "POD_NAME",
									Value: jobName,
								},
								{
									Name:  "CLUSTER_NAME",
									Value: cluster.Name,
								},
								{
									Name:  "NAMESPACE",
									Value: cluster.Namespace,
								},
								{
									Name: "AWS_ACCESS_KEY_ID",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &backup.Status.S3Credentials.AccessKeyIDReference,
									},
								},
								{
									Name: "AWS_SECRET_ACCESS_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &backup.Status.S3Credentials.SecretAccessKeyReference,
									},
								},
								{
									Name:  "PGHOST",
									Value: "/var/run/postgresql",
								},
							},
							Command: initCommand,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "pgdata",
									MountPath: "/var/lib/postgresql/data",
								},
								{
									Name:      "config",
									MountPath: "/etc/configuration",
								},
								{
									Name:      "superuser-secret",
									MountPath: "/etc/superuser-secret",
								},
								{
									Name:      "controller",
									MountPath: "/controller",
								},
								{
									Name:      "socket",
									MountPath: "/var/run/postgresql",
								},
							},
						},
					},
					Volumes:            createPostgresVolumes(cluster, podName),
					SecurityContext:    CreatePostgresSecurityContext(cluster.GetPostgresUID(), cluster.GetPostgresGID()),
					Affinity:           CreateAffinitySection(cluster.Name, cluster.Spec.Affinity),
					ServiceAccountName: cluster.Name,
					RestartPolicy:      corev1.RestartPolicyNever,
				},
			},
		},
	}

	return job
}

// JoinReplicaInstance create a new PostgreSQL node, copying the contents from another Pod
func JoinReplicaInstance(cluster v1alpha1.Cluster, nodeSerial int32) *batchv1.Job {
	podName := fmt.Sprintf("%s-%v", cluster.Name, nodeSerial)
	jobName := fmt.Sprintf("%s-%v-join", cluster.Name, nodeSerial)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Hostname:  jobName,
					Subdomain: cluster.GetServiceAnyName(),
					InitContainers: []corev1.Container{
						{
							Name:  "bootstrap-controller",
							Image: versions.GetDefaultOperatorImageName(),
							Command: []string{
								"/manager",
								"bootstrap",
								"/controller/manager",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "controller",
									MountPath: "/controller",
								},
								{
									Name:      "socket",
									MountPath: "/var/run/postgresql",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "PGHOST",
									Value: "/var/run/postgresql",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "bootstrap-replica",
							Image: cluster.GetImageName(),
							Env: []corev1.EnvVar{
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
								{
									Name:  "POD_NAME",
									Value: podName,
								},
								{
									Name:  "CLUSTER_NAME",
									Value: cluster.Name,
								},
								{
									Name:  "NAMESPACE",
									Value: cluster.Namespace,
								},
								{
									Name:  "PGHOST",
									Value: "/var/run/postgresql",
								},
							},
							Command: []string{
								"/controller/manager",
								"instance",
								"join",
								"-parent-node", cluster.GetServiceReadWriteName(),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "pgdata",
									MountPath: "/var/lib/postgresql/data",
								},
								{
									Name:      "controller",
									MountPath: "/controller",
								},
								{
									Name:      "socket",
									MountPath: "/var/run/postgresql",
								},
							},
						},
					},
					Volumes:            createPostgresVolumes(cluster, podName),
					SecurityContext:    CreatePostgresSecurityContext(cluster.GetPostgresUID(), cluster.GetPostgresGID()),
					Affinity:           CreateAffinitySection(cluster.Name, cluster.Spec.Affinity),
					ServiceAccountName: cluster.Name,
					RestartPolicy:      corev1.RestartPolicyNever,
				},
			},
		},
	}

	return job
}

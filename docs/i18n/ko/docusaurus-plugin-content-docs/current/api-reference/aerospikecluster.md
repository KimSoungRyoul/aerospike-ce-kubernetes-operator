---
sidebar_position: 1
title: AerospikeCluster API 레퍼런스
---

# AerospikeCluster API 레퍼런스

이 페이지는 `AerospikeCluster` Custom Resource Definition(CRD) 타입을 문서화합니다.

**API Group:** `acko.io`
**API Version:** `v1alpha1`
**Kind:** `AerospikeCluster`
**Short Names:** `asc`

---

## AerospikeCluster

AerospikeCluster는 `aerospikeclusters` API의 스키마입니다. Aerospike Community Edition 클러스터의 라이프사이클을 관리합니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `apiVersion` | string | `acko.io/v1alpha1` |
| `kind` | string | `AerospikeCluster` |
| `metadata` | [ObjectMeta](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/) | 표준 객체 메타데이터 |
| `spec` | [AerospikeClusterSpec](#aerospikeclusterspec) | 클러스터의 원하는 상태 |
| `status` | [AerospikeClusterStatus](#aerospikeclusterstatus) | 클러스터의 관측된 상태 |

---

## AerospikeClusterSpec

Aerospike CE 클러스터의 원하는 상태를 정의합니다.

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|---|---|---|---|---|
| `size` | int32 | 예 | — | Aerospike 파드 수. CE 최대: 8. |
| `image` | string | 예 | — | Aerospike CE 컨테이너 이미지 (예: `aerospike:ce-8.1.1.1`). |
| `aerospikeConfig` | [AerospikeConfigSpec](#aerospikeconfigspec) | 아니요 | — | Aerospike 설정 맵, `aerospike.conf`로 변환됨. |
| `storage` | [AerospikeStorageSpec](#aerospikestoragespec) | 아니요 | — | Aerospike 파드의 볼륨 정의. |
| `rackConfig` | [RackConfig](#rackconfig) | 아니요 | — | 랙 인식 배포 토폴로지. |
| `aerospikeNetworkPolicy` | [AerospikeNetworkPolicy](#aerospikenetworkpolicy) | 아니요 | — | 클라이언트 접근 네트워크 설정. |
| `podSpec` | [AerospikePodSpec](#aerospikepodspec) | 아니요 | — | 파드 레벨 설정. |
| `aerospikeAccessControl` | [AerospikeAccessControlSpec](#aerospikeaccesscontrolspec) | 아니요 | — | ACL 역할 및 사용자. |
| `monitoring` | [AerospikeMonitoringSpec](#aerospikemonitoringspec) | 아니요 | — | Prometheus 모니터링 설정. |
| `networkPolicyConfig` | [NetworkPolicyConfig](#networkpolicyconfig) | 아니요 | — | 자동 NetworkPolicy 생성. |
| `bandwidthConfig` | [BandwidthConfig](#bandwidthconfig) | 아니요 | — | CNI 대역폭 어노테이션. |
| `enableDynamicConfigUpdate` | *bool | 아니요 | — | `set-config`를 통한 런타임 설정 변경 활성화. |
| `rollingUpdateBatchSize` | *int32 | 아니요 | `1` | 롤링 업데이트 시 동시 재시작 파드 수. |
| `disablePDB` | *bool | 아니요 | `false` | PodDisruptionBudget 생성 비활성화. |
| `maxUnavailable` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | 아니요 | `1` | 중단 중 최대 비가용 파드 수. |
| `paused` | *bool | 아니요 | `false` | true이면 재조정 중지. |
| `seedsFinderServices` | [SeedsFinderServices](#seedsfinderservices) | 아니요 | — | 시드 디스커버리용 LoadBalancer 서비스. |
| `k8sNodeBlockList` | []string | 아니요 | — | 스케줄링에서 제외할 노드 이름. |
| `operations` | [][OperationSpec](#operationspec) | 아니요 | — | 온디맨드 오퍼레이션 (WarmRestart, PodRestart). 동시에 최대 1개. |
| `validationPolicy` | [ValidationPolicySpec](#validationpolicyspec) | 아니요 | — | 웹훅 검증 동작 제어. |
| `headlessService` | [AerospikeServiceSpec](#aerospikeservicespec) | 아니요 | — | Headless 서비스 커스텀 메타데이터. |
| `podService` | [AerospikeServiceSpec](#aerospikeservicespec) | 아니요 | — | Pod별 서비스 커스텀 메타데이터. 설정 시 파드마다 개별 Service 생성. |
| `enableRackIDOverride` | *bool | 아니요 | `false` | 파드 어노테이션을 통한 동적 랙 ID 할당 활성화. |
| `templateRef` | [TemplateRef](#templateref) | 아니요 | — | `AerospikeClusterTemplate` 참조. 설정 시 템플릿 스펙이 생성 시점에 스냅샷으로 저장됨. |
| `overrides` | [AerospikeClusterTemplateSpec](./aerospikeclustertemplate#aerospikeclustertemplatespec) | 아니요 | — | 참조된 템플릿을 오버라이드하는 필드. 병합 우선순위: overrides > template > 오퍼레이터 기본값. |

---

## TemplateRef

같은 네임스페이스의 `AerospikeClusterTemplate` 참조입니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `name` | string | 예 | `AerospikeClusterTemplate` 리소스 이름 |

---

## TemplateSnapshotStatus

템플릿이 해결된 후 `status.templateSnapshot`에 기록됩니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `name` | string | 참조된 템플릿 이름 |
| `resourceVersion` | string | 스냅샷 시점의 템플릿 ResourceVersion |
| `snapshotTimestamp` | Time | 스냅샷이 촬영된 시점 |
| `synced` | bool | 클러스터가 최신 템플릿 버전을 사용하는지 여부. 스냅샷 이후 템플릿 변경 시 `false`로 설정. |
| `spec` | [AerospikeClusterTemplateSpec](./aerospikeclustertemplate#aerospikeclustertemplatespec) | 스냅샷 시점의 해결된 템플릿 스펙. |

---

## AerospikeConfigSpec

비구조화된 JSON/YAML 객체로 Aerospike 설정을 보유합니다. 오퍼레이터가 이를 `aerospike.conf` 형식으로 변환합니다.

YAML에서 Aerospike 설정을 직접 작성합니다:

```yaml
aerospikeConfig:
  service:
    cluster-name: my-cluster
    proto-fd-max: 15000
  network:
    service:
      port: 3000
    heartbeat:
      mode: mesh
      port: 3002
    fabric:
      port: 3001
  namespaces:
    - name: testns
      replication-factor: 2
      storage-engine:
        type: device
        file: /opt/aerospike/data/testns.dat
        filesize: 4294967296
  logging:
    - name: /var/log/aerospike/aerospike.log
      context: any info
```

---

## AerospikeClusterStatus

Aerospike CE 클러스터의 관측된 상태입니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `phase` | string | 클러스터 단계: `InProgress`, `Completed`, `Error`, `ScalingUp`, `ScalingDown`, `RollingRestart`, `ACLSync`, `Paused`, `Deleting`. |
| `size` | int32 | 현재 클러스터 크기. |
| `conditions` | [][Condition](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/conditions/) | 클러스터 상태의 최신 관측. |
| `pods` | map[string][AerospikePodStatus](#aerospikepodstatus) | 파드별 상태 정보, 파드 이름으로 키 지정. |
| `observedGeneration` | int64 | 컨트롤러가 관측한 가장 최근 세대. |
| `selector` | string | HPA 호환을 위한 레이블 셀렉터 문자열. |
| `aerospikeConfig` | [AerospikeConfigSpec](#aerospikeconfigspec) | 마지막으로 적용된 Aerospike 설정. |
| `operationStatus` | [OperationStatus](#operationstatus) | 현재 온디맨드 오퍼레이션 상태. |
| `phaseReason` | string | 현재 단계의 사람이 읽을 수 있는 설명 (예: "Rolling restart in progress for rack 1"). |
| `appliedSpec` | [AerospikeClusterSpec](#aerospikeclusterspec) | 마지막으로 성공적으로 재조정된 스펙의 사본. 설정 드리프트 감지용. |
| `aerospikeClusterSize` | int32 | `asinfo`로 보고된 Aerospike 클러스터 크기. 스플릿 브레인이나 롤링 리스타트 중 K8s 파드 수와 다를 수 있음. |
| `operatorVersion` | string | 이 클러스터를 마지막으로 재조정한 오퍼레이터 버전. |
| `pendingRestartPods` | []string | 현재 롤링 리스타트에서 재시작 대기 중인 파드. 완료 시 비워짐. |
| `lastReconcileTime` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | 마지막 성공적인 재조정의 타임스탬프. |
| `templateSnapshot` | [TemplateSnapshotStatus](#templatesnapshotstatus) | 마지막 동기화 시점의 해결된 템플릿 스펙. |
| `failedReconcileCount` | int32 | 연속 재조정 실패 횟수. 성공 시 0으로 초기화. 서킷 브레이커 임계값(기본 10)을 초과하면 오퍼레이터가 지수 백오프. |
| `lastReconcileError` | string | 가장 최근 실패한 재조정의 오류 메시지. 성공 시 비워짐. |
| `migrationStatus` | [MigrationStatus](#migrationstatus) | 클러스터 레벨 데이터 마이그레이션 진행 상태. 각 재조정 시 Aerospike 노드의 파티션 마이그레이션 통계를 조회하여 업데이트. |

---

## MigrationStatus

클러스터의 현재 데이터 마이그레이션 상태를 추적합니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `inProgress` | bool | 데이터 마이그레이션이 현재 진행 중인지 여부. |
| `remainingPartitions` | int64 | 전체 노드에서 아직 마이그레이션되어야 할 파티션 총 수. `0`이면 마이그레이션 완료. |
| `lastChecked` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | 마지막 마이그레이션 상태 확인 타임스탬프. |

---

## Condition Types

오퍼레이터가 `status.conditions`에서 관리하는 조건 타입입니다:

| 타입 | 설명 |
|---|---|
| `Available` | 최소 하나의 파드가 요청을 처리할 준비가 됨. |
| `Ready` | 모든 원하는 파드가 실행 중이고 준비됨. |
| `ConfigApplied` | 모든 파드에 원하는 Aerospike 설정이 적용됨. |
| `ACLSynced` | ACL 역할과 사용자가 클러스터와 동기화됨. |
| `MigrationComplete` | 보류 중인 데이터 마이그레이션이 없음. |
| `ReconciliationPaused` | 사용자에 의해 재조정이 일시 중지됨 (`spec.paused: true`). |

---

## AerospikePodStatus

파드별 상태 정보입니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `podIP` | string | 파드 IP 주소. |
| `hostIP` | string | 호스트 노드 IP 주소. |
| `image` | string | 파드에서 실행 중인 컨테이너 이미지. |
| `podPort` | int32 | 파드의 Aerospike 서비스 포트. |
| `servicePort` | int32 | 노드/LB로 노출된 Aerospike 서비스 포트. |
| `rack` | int | 이 파드에 할당된 랙 ID. |
| `initializedVolumes` | []string | 초기화된 볼륨 목록. |
| `isRunningAndReady` | bool | 파드가 실행 중이고 준비되었는지 여부. |
| `configHash` | string | 적용된 설정의 SHA256 해시. |
| `podSpecHash` | string | 파드 템플릿 스펙의 해시. |
| `dynamicConfigStatus` | string | 동적 설정 업데이트 결과: `Applied`, `Failed`, `Pending`, 또는 빈 문자열. |
| `dirtyVolumes` | []string | 초기화 또는 정리가 필요한 볼륨. |
| `nodeID` | string | Aerospike가 할당한 노드 식별자 (예: `BB9020012AC4202`). 접근 불가 시 빈 문자열. |
| `clusterName` | string | 노드가 보고한 Aerospike 클러스터 이름. |
| `accessEndpoints` | []string | `asinfo "service"`를 통한 직접 클라이언트 접근용 네트워크 엔드포인트 (`host:port`). |
| `readinessGateSatisfied` | bool | `acko.io/aerospike-ready` 게이트가 `True`인지 여부. `readinessGateEnabled=true`일 때만 유효. |
| `lastRestartReason` | [RestartReason](#restartreason) | 오퍼레이터에 의해 파드가 마지막으로 재시작된 이유. |
| `lastRestartTime` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | 오퍼레이터에 의해 파드가 마지막으로 재시작된 시점. |
| `unstableSince` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | 이 파드가 처음 NotReady가 된 시점. Ready로 돌아가면 `nil`로 초기화. |
| `migratingPartitions` | *int64 | 이 파드가 현재 마이그레이션 중인 파티션 수. 노드의 `migrate_partitions_remaining` 통계를 조회하여 채워짐. 접근 불가 시 `nil`. |

---

## RestartReason

오퍼레이터에 의해 파드가 재시작된 이유를 설명합니다.

| 값 | 설명 |
|---|---|
| `ConfigChanged` | Aerospike 설정 변경으로 인한 콜드 리스타트. |
| `ImageChanged` | 파드 이미지 업데이트. |
| `PodSpecChanged` | 파드 스펙(리소스, 환경변수 등) 변경. |
| `ManualRestart` | 온디맨드 파드 리스타트 (`OperationPodRestart`). |
| `WarmRestart` | 온디맨드 또는 롤링 웜 리스타트 (SIGUSR1). |

---

## AerospikeStorageSpec

Aerospike 파드의 스토리지 볼륨을 정의합니다.

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|---|---|---|---|---|
| `volumes` | [][VolumeSpec](#volumespec) | 아니요 | — | 연결할 볼륨 목록. |
| `cleanupThreads` | int32 | 아니요 | `1` | 볼륨 정리/초기화 최대 스레드 수. |
| `filesystemVolumePolicy` | [AerospikeVolumePolicy](#aerospikevolumepolicy) | 아니요 | — | 파일시스템 모드 영구 볼륨의 기본 정책. 볼륨별 설정이 우선. |
| `blockVolumePolicy` | [AerospikeVolumePolicy](#aerospikevolumepolicy) | 아니요 | — | 블록 모드 영구 볼륨의 기본 정책. 볼륨별 설정이 우선. |
| `localStorageClasses` | []string | 아니요 | — | 로컬 스토리지를 사용하는 StorageClass 이름 (예: `local-path`). 파드 재시작 시 특수 처리 필요. |
| `deleteLocalStorageOnRestart` | *bool | 아니요 | — | 파드 재시작 전 로컬 PVC를 삭제하여 새 노드에서 재프로비저닝. |

---

## AerospikeVolumePolicy

영구 볼륨 카테고리(파일시스템 또는 블록)의 기본 정책입니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `initMethod` | string | `none` | 이 볼륨 카테고리의 기본 초기화 방법. |
| `wipeMethod` | string | `none` | 이 볼륨 카테고리의 기본 와이프 방법. |
| `cascadeDelete` | *bool | `nil` | CR 삭제 시 PVC 삭제 여부. |

---

## VolumeSpec

단일 볼륨 연결을 정의합니다.

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|---|---|---|---|---|
| `name` | string | 예 | — | 볼륨 이름. |
| `source` | [VolumeSource](#volumesource) | 예 | — | 볼륨 소스 (PVC, emptyDir, secret, configMap, hostPath). |
| `aerospike` | [AerospikeVolumeAttachment](#aerospikevolumeattachment) | 아니요 | — | Aerospike 컨테이너의 마운트 경로. |
| `sidecars` | [][VolumeAttachment](#volumeattachment) | 아니요 | — | 사이드카 컨테이너의 볼륨 마운트. |
| `initContainers` | [][VolumeAttachment](#volumeattachment) | 아니요 | — | 초기화 컨테이너의 볼륨 마운트. |
| `initMethod` | string | 아니요 | `none` | 초기화 방법: `none`, `deleteFiles`, `dd`, `blkdiscard`, `headerCleanup`. |
| `wipeMethod` | string | 아니요 | `none` | 더티 볼륨 와이프 방법: `none`, `deleteFiles`, `dd`, `blkdiscard`, `headerCleanup`, `blkdiscardWithHeaderCleanup`. |
| `cascadeDelete` | *bool | 아니요 | `nil` | CR 삭제 시 PVC 삭제. `nil`이면 글로벌 볼륨 정책으로 폴백. |

---

## VolumeSource

볼륨 데이터 소스를 설명합니다. 정확히 하나의 필드만 설정해야 합니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `persistentVolume` | [PersistentVolumeSpec](#persistentvolumespec) | PVC 생성. |
| `emptyDir` | [EmptyDirVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#emptyDir) | emptyDir 사용. |
| `secret` | [SecretVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#secret) | Kubernetes Secret 사용. |
| `configMap` | [ConfigMapVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#configMap) | Kubernetes ConfigMap 사용. |
| `hostPath` | [HostPathVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#hostPath) | 호스트 노드의 경로 사용. |

---

## PersistentVolumeSpec

PVC 템플릿을 정의합니다.

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|---|---|---|---|---|
| `storageClass` | string | 아니요 | — | StorageClass 이름. |
| `volumeMode` | string | 아니요 | `Filesystem` | `Filesystem` 또는 `Block`. |
| `size` | string | 예 | — | 스토리지 크기 (예: `10Gi`). |
| `accessModes` | []string | 아니요 | — | 접근 모드 (예: `ReadWriteOnce`). |
| `selector` | [LabelSelector](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/label-selector/) | 아니요 | — | PV 바인딩용 레이블 셀렉터. |
| `metadata` | [AerospikeObjectMeta](#aerospikeobjectmeta) | 아니요 | — | PVC의 커스텀 레이블 및 어노테이션. |

---

## AerospikeVolumeAttachment

볼륨이 Aerospike 컨테이너에 마운트되는 방식을 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `path` | string | 예 | 컨테이너의 마운트 경로. |
| `readOnly` | bool | 아니요 | 볼륨을 읽기 전용으로 마운트. |
| `subPath` | string | 아니요 | 볼륨의 특정 하위 경로만 마운트. |
| `subPathExpr` | string | 아니요 | 환경 변수를 사용한 확장 경로. `subPath`와 상호 배타적. |
| `mountPropagation` | [MountPropagationMode](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation) | 아니요 | 마운트 전파 방식: `None`, `HostToContainer`, `Bidirectional`. |

---

## VolumeAttachment

사이드카 또는 초기화 컨테이너의 볼륨 마운트를 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `containerName` | string | 예 | 대상 컨테이너 이름. |
| `path` | string | 예 | 컨테이너의 마운트 경로. |
| `readOnly` | bool | 아니요 | 볼륨을 읽기 전용으로 마운트. |
| `subPath` | string | 아니요 | 볼륨의 특정 하위 경로만 마운트. |
| `subPathExpr` | string | 아니요 | 환경 변수를 사용한 확장 경로. `subPath`와 상호 배타적. |
| `mountPropagation` | [MountPropagationMode](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation) | 아니요 | 마운트 전파 방식: `None`, `HostToContainer`, `Bidirectional`. |

---

## AerospikeNetworkPolicy

네트워크 접근 설정을 정의합니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `accessType` | string | `pod` | 클라이언트 접근 타입: `pod`, `hostInternal`, `hostExternal`, `configuredIP`. |
| `alternateAccessType` | string | `pod` | 대체 접근 타입. |
| `fabricType` | string | `pod` | 패브릭(노드 간) 네트워크 타입. |
| `customAccessNetworkNames` | []string | — | `configuredIP` 접근용 네트워크 이름. |
| `customAlternateAccessNetworkNames` | []string | — | `configuredIP` 대체 접근용 네트워크 이름. |
| `customFabricNetworkNames` | []string | — | `configuredIP` 패브릭용 네트워크 이름. |

---

## SeedsFinderServices

LoadBalancer를 통한 외부 시드 디스커버리를 설정합니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `loadBalancer` | [LoadBalancerSpec](#loadbalancerspec) | LoadBalancer 서비스 설정. |

---

## LoadBalancerSpec

LoadBalancer 서비스를 정의합니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `annotations` | map[string]string | — | 서비스 어노테이션. |
| `labels` | map[string]string | — | 서비스 레이블. |
| `externalTrafficPolicy` | string | — | `Cluster` 또는 `Local`. |
| `port` | int32 | `3000` | 외부 포트. |
| `targetPort` | int32 | `3000` | 컨테이너 대상 포트. |
| `loadBalancerSourceRanges` | []string | — | 허용된 소스 CIDR. |

---

## AerospikePodSpec

Aerospike 파드의 파드 레벨 커스터마이징입니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `aerospikeContainer` | [AerospikeContainerSpec](#aerospikecontainerspec) | Aerospike 컨테이너 커스터마이징. |
| `sidecars` | [][Container](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#Container) | 사이드카 컨테이너. |
| `initContainers` | [][Container](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#Container) | 추가 초기화 컨테이너. |
| `imagePullSecrets` | [][LocalObjectReference](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/local-object-reference/) | 이미지 풀 시크릿. |
| `nodeSelector` | map[string]string | 스케줄링용 노드 레이블. |
| `tolerations` | [][Toleration](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | 파드 toleration. |
| `affinity` | [Affinity](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | 어피니티/안티-어피니티 규칙. |
| `securityContext` | [PodSecurityContext](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#security-context) | 파드 레벨 보안 속성. |
| `serviceAccountName` | string | ServiceAccount 이름. |
| `dnsPolicy` | string | 파드 DNS 정책. |
| `hostNetwork` | bool | 호스트 네트워킹 활성화. |
| `multiPodPerHost` | *bool | 같은 노드에 여러 파드 허용. |
| `terminationGracePeriodSeconds` | *int64 | 파드 종료 유예 기간. |
| `topologySpreadConstraints` | [][TopologySpreadConstraint](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | 토폴로지 도메인 간 파드 분산 방식. |
| `podManagementPolicy` | string | StatefulSet 파드 관리: `OrderedReady` (기본) 또는 `Parallel`. |
| `metadata` | [AerospikePodMetadata](#aerospikepodmetadata) | 추가 파드 레이블/어노테이션. |
| `readinessGateEnabled` | *bool | 커스텀 readiness gate `acko.io/aerospike-ready` 활성화. Aerospike가 클러스터 mesh에 참여하고 마이그레이션이 완료될 때까지 Service 엔드포인트에서 제외. |
| `priorityClassName` | string | 파드 스케줄링 우선순위 및 선점(preemption)을 위한 PriorityClass 이름. 클러스터에 해당 PriorityClass가 미리 존재해야 합니다. |

---

## AerospikeContainerSpec

Aerospike 서버 컨테이너를 커스터마이징합니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `resources` | [ResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | CPU/메모리 요청 및 제한. |
| `securityContext` | [SecurityContext](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#security-context-1) | 컨테이너 레벨 보안 속성. |

---

## AerospikePodMetadata

파드의 추가 레이블 및 어노테이션입니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `labels` | map[string]string | 추가 파드 레이블. |
| `annotations` | map[string]string | 추가 파드 어노테이션. |

---

## RackConfig

랙 인식 배포 설정을 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `racks` | [][Rack](#rack) | 예 | 랙 정의 목록 (최소 1). |
| `namespaces` | []string | 아니요 | 랙 인식 Aerospike 네임스페이스 이름. |
| `scaleDownBatchSize` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | 아니요 | 랙당 동시 스케일 다운 파드 수. 정수 또는 퍼센트 문자열 (예: `"25%"`). 기본값: 1. |
| `maxIgnorablePods` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | 아니요 | 재조정 중 무시할 수 있는 최대 Pending/Failed 파드 수. |
| `rollingUpdateBatchSize` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | 아니요 | 랙당 동시 재시작 파드 수. 정수 또는 퍼센트 문자열. `spec.rollingUpdateBatchSize`보다 우선. |

---

## Rack

클러스터 토폴로지의 단일 랙을 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `id` | int | 예 | 고유 랙 식별자 (>= 1). 랙 ID 0은 기본 랙용으로 예약됨. |
| `zone` | string | 아니요 | 존 레이블 값 (`topology.kubernetes.io/zone`). |
| `region` | string | 아니요 | 리전 레이블 값 (`topology.kubernetes.io/region`). |
| `nodeName` | string | 아니요 | 특정 노드로 제한. |
| `rackLabel` | string | 아니요 | 랙 어피니티용 커스텀 라벨. `acko.io/rack=<rackLabel>` 노드에 스케줄링. 랙 간 고유해야 함. |
| `revision` | string | 아니요 | 제어된 랙 마이그레이션용 버전 식별자. |
| `aerospikeConfig` | [AerospikeConfigSpec](#aerospikeconfigspec) | 아니요 | 랙별 Aerospike 설정 오버라이드. |
| `storage` | [AerospikeStorageSpec](#aerospikestoragespec) | 아니요 | 랙별 스토리지 오버라이드. |
| `podSpec` | [RackPodSpec](#rackpodspec) | 아니요 | 랙별 파드 스케줄링 오버라이드. |

---

## RackPodSpec

랙 레벨 파드 스케줄링 오버라이드입니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `affinity` | [Affinity](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | 랙 레벨 어피니티 오버라이드. |
| `tolerations` | [][Toleration](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | 랙 레벨 toleration 오버라이드. |
| `nodeSelector` | map[string]string | 랙 레벨 노드 셀렉터 오버라이드. |

---

## AerospikeAccessControlSpec

ACL 설정을 정의합니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `roles` | [][AerospikeRoleSpec](#aerospikerolespec) | Aerospike 역할 정의. |
| `users` | [][AerospikeUserSpec](#aerospikeuserspec) | Aerospike 사용자 정의. |
| `adminPolicy` | [AerospikeClientAdminPolicy](#aerospikeclientadminpolicy) | 관리자 클라이언트 타임아웃 정책. |

---

## AerospikeRoleSpec

Aerospike 역할을 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `name` | string | 예 | 역할 이름. |
| `privileges` | []string | 예 | 권한 문자열: `read`, `write`, `read-write`, `read-write-udf`, `sys-admin`, `user-admin`, `data-admin`, `truncate`. 네임스페이스 범위 지원 (예: `read-write.testns`). |
| `whitelist` | []string | 아니요 | 허용된 CIDR 범위. |

---

## AerospikeUserSpec

Aerospike 사용자를 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `name` | string | 예 | 사용자 이름. |
| `secretName` | string | 예 | 비밀번호를 포함하는 Kubernetes Secret 이름 (키: `password`). |
| `roles` | []string | 예 | 할당된 역할 이름 (최소 1). |

---

## AerospikeClientAdminPolicy

관리자 클라이언트 타임아웃 설정입니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `timeout` | int | `2000` | 관리자 작업 타임아웃 (밀리초). |

---

## AerospikeMonitoringSpec

Prometheus 모니터링 설정입니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `enabled` | bool | `false` | Prometheus 익스포터 사이드카 활성화. |
| `exporterImage` | string | `aerospike/aerospike-prometheus-exporter:1.16.1` | 익스포터 컨테이너 이미지. |
| `port` | int32 | `9145` | 메트릭 포트. |
| `resources` | [ResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | — | 익스포터 리소스 제한. |
| `env` | [][EnvVar](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#environment-variables) | — | 익스포터 컨테이너의 추가 환경 변수. |
| `metricLabels` | map[string]string | — | 모든 익스포트 메트릭에 추가되는 커스텀 레이블. `METRIC_LABELS` 환경 변수로 전달. |
| `serviceMonitor` | [ServiceMonitorSpec](#servicemonitorspec) | — | ServiceMonitor 설정. |
| `prometheusRule` | [PrometheusRuleSpec](#prometheusrulespec) | — | 클러스터 알림을 위한 PrometheusRule 설정. |

---

## ServiceMonitorSpec

Prometheus Operator를 위한 ServiceMonitor 설정입니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `enabled` | bool | `false` | ServiceMonitor 리소스 생성. |
| `interval` | string | `30s` | 스크래핑 주기. |
| `labels` | map[string]string | — | ServiceMonitor 디스커버리용 추가 레이블. |

---

## PrometheusRuleSpec

Aerospike 클러스터 알림을 위한 PrometheusRule 설정입니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `enabled` | bool | `false` | PrometheusRule 리소스 생성. |
| `labels` | map[string]string | — | PrometheusRule 디스커버리용 추가 레이블. |
| `customRules` | []JSON | — | 기본 알림(NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage)을 대체하는 커스텀 규칙 그룹. 각 항목에 `name`과 `rules` 필드가 필수. |

---

## NetworkPolicyConfig

자동 NetworkPolicy 생성입니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `enabled` | bool | `false` | NetworkPolicy 생성 활성화. |
| `type` | string | `kubernetes` | 정책 타입: `kubernetes` 또는 `cilium`. |

---

## BandwidthConfig

CNI 트래픽 셰이핑을 위한 대역폭 어노테이션입니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `ingress` | string | 최대 인그레스 대역폭 (예: `1Gbps`, `500Mbps`). |
| `egress` | string | 최대 이그레스 대역폭 (예: `1Gbps`, `500Mbps`). |

---

## OperationSpec

클러스터 파드에 대한 온디맨드 오퍼레이션을 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `kind` | string | 예 | 오퍼레이션 타입: `WarmRestart` (SIGUSR1) 또는 `PodRestart` (삭제/재생성). |
| `id` | string | 예 | 고유 오퍼레이션 식별자 (1-20자). |
| `podList` | []string | 아니요 | 대상 파드 이름 목록. 비우면 전체 파드 대상. |

---

## OperationStatus

온디맨드 오퍼레이션의 상태를 추적합니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `id` | string | 오퍼레이션 식별자. |
| `kind` | string | 오퍼레이션 타입: `WarmRestart` 또는 `PodRestart`. |
| `phase` | string | 오퍼레이션 단계: `InProgress`, `Completed`, 또는 `Error`. |
| `completedPods` | []string | 오퍼레이션이 완료된 파드 목록. |
| `failedPods` | []string | 오퍼레이션이 실패한 파드 목록. |

---

## ValidationPolicySpec

웹훅 검증 동작을 제어합니다.

| 필드 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `skipWorkDirValidate` | bool | `false` | Aerospike 작업 디렉토리가 영구 스토리지에 있는지 검증을 건너뜁니다. |

---

## AerospikeServiceSpec

Kubernetes Service의 커스텀 메타데이터를 정의합니다.

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `metadata` | [AerospikeObjectMeta](#aerospikeobjectmeta) | 아니요 | 서비스의 커스텀 어노테이션 및 레이블. |

---

## AerospikeObjectMeta

Kubernetes 객체의 커스텀 메타데이터입니다.

| 필드 | 타입 | 설명 |
|---|---|---|
| `annotations` | map[string]string | 커스텀 어노테이션. |
| `labels` | map[string]string | 커스텀 레이블. |

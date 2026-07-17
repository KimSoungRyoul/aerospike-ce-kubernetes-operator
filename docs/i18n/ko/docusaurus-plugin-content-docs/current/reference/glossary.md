---
sidebar_position: 2
title: 용어 사전
---

# 용어 사전

Aerospike, Kubernetes, ACKO에서 서로 다른 의미로 쓰이는 용어를 이 표에서 구분합니다.

## 오퍼레이터 & CRD 개념

| 용어 | 정의 |
|------|------|
| **ACKO** | **A**erospike **C**ommunity Edition **K**ubernetes **O**perator. `acko.io` API 그룹의 `AerospikeCluster`와 `AerospikeClusterTemplate` 리소스를 관리합니다. |
| **AerospikeCluster** | Aerospike CE 클러스터 배포를 나타내는 주요 Custom Resource (CRD Kind). `apiVersion: acko.io/v1alpha1`. 단축 이름: `asc`. |
| **AerospikeClusterTemplate** | 클러스터를 위한 재사용 가능한 설정 프로필을 제공하는 CRD. `spec.templateRef`로 참조합니다. 단축 이름: `asct`. |
| **Rack** | Aerospike 클러스터 내의 논리적 장애 도메인. ACKO에서 각 랙은 하나의 StatefulSet과 하나의 ConfigMap (`<clusterName>-<rackID>` 패턴)에 매핑됩니다. Rack ID는 사용자가 지정하는 양의 정수이며, ID 0은 내부적으로 예약되어 있습니다. |
| **CR / Custom Resource** | CRD의 인스턴스. 예를 들어, 네임스페이스 내의 특정 `AerospikeCluster` 객체가 CR입니다. |

## Aerospike vs Kubernetes 용어 구분

| 용어 | Aerospike에서 | Kubernetes에서 |
|------|-------------|---------------|
| **Node** | 단일 Aerospike 서버 프로세스 (`asd` 데몬). ACKO에서는 Pod당 하나의 Aerospike 노드가 실행됩니다 (1:1 매핑). | K8s 클러스터의 워커 머신 (물리적 또는 가상 VM). |
| **Namespace** | Aerospike 내부의 데이터 파티션 — 데이터베이스와 유사. CE는 최대 2개까지 지원. `spec.aerospikeConfig.namespaces`에서 설정. | K8s 리소스의 격리 경계. CR이 위치하는 네임스페이스. |
| **Cluster** | 단일 분산 데이터베이스를 구성하는 Aerospike 노드들의 집합. 하나의 `AerospikeCluster` CR로 표현됩니다. | 전체 Kubernetes 시스템 (컨트롤 플레인 + 워커 노드). |

## Pod 및 재시작 개념

| 용어 | 정의 |
|------|------|
| **Pod** | 정확히 하나의 Aerospike 서버 컨테이너 (`aerospike-server`)와 선택적 사이드카 (예: Prometheus 익스포터)를 실행하는 Kubernetes Pod. |
| **Warm Restart** | `asd` 프로세스에 `SIGUSR1`을 전송 — 인메모리 데이터를 잃지 않고 서버를 재시작합니다 (CE 8.x+). 콜드 재시작보다 빠릅니다. |
| **Cold Restart** | Pod를 삭제하고 재생성합니다. Aerospike 프로세스가 새로 시작되며 인메모리 데이터가 손실됩니다. Pod 스펙 변경에 필요합니다. |
| **Rolling Restart** | 오퍼레이터가 가용성을 유지하면서 설정 또는 이미지 변경 시 `spec.rollingUpdateBatchSize`에 의해 제어되는 배치 단위로 Pod를 순차적으로 재시작합니다. |

## 설정 개념

| 용어 | 정의 |
|------|------|
| **aerospikeConfig** | `spec.aerospikeConfig` 필드 — `aerospike.conf`에 직접 매핑되는 자유 형식 맵. 오퍼레이터가 이를 Aerospike의 텍스트 설정 형식으로 변환합니다. |
| **Dynamic Config Update** | Pod를 재시작하지 않고 Aerospike의 `set-config` 명령을 통해 런타임에 설정 변경을 적용합니다. `spec.enableDynamicConfigUpdate: true`로 활성화. |
| **Config Hash** | 각 Pod의 어노테이션 (`acko.io/config-hash`)에 저장되는 유효 Aerospike 설정의 SHA-256 해시. 오퍼레이터가 어떤 Pod를 재시작해야 하는지 감지하는 데 사용합니다. |
| **PodSpec Hash** | `acko.io/podspec-hash`로 저장되는 Pod 스펙의 SHA-256 해시. 변경 시 콜드 재시작 (Pod 삭제 + 재생성)을 트리거합니다. |

## 주요 약어

| 약어 | 의미 |
|---|---|
| `asc` | `aerospikeclusters`의 단축 이름 (kubectl 별칭) |
| `asct` | `aerospikeclustertemplates`의 단축 이름 |
| `asd` | Aerospike 서버 데몬 — Aerospike 데이터베이스 프로세스 |
| `asinfo` | 노드 상태 조회용 Aerospike info CLI 도구 |
| `asadm` | 관리 작업용 Aerospike admin CLI 도구 |
| `PDB` | PodDisruptionBudget — 동시 Pod 중단을 제한하는 K8s 객체 |
| `PVC` | PersistentVolumeClaim — K8s 스토리지 요청 |
| `ACL` | Access Control List — Aerospike 사용자/역할 권한 |

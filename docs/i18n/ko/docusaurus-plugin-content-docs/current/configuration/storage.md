---
sidebar_position: 1
title: 스토리지 설정
---

# 스토리지 설정

`spec.storage`에서 네임스페이스 데이터, secondary index, 로그에 쓸 볼륨의 provisioning, 초기화, 정리 방식을 지정합니다.

---

## 글로벌 볼륨 정책

각 볼륨에서 별도로 override하지 않으면 전역 정책이 모든 볼륨에 적용됩니다.

### filesystemVolumePolicy

`volumeMode: Filesystem`(기본값)인 PVC 볼륨에 적용됩니다.

| 필드 | 옵션 | 설명 |
|------|------|------|
| `initMethod` | `none`, `dd`, `deleteFiles`, `blkdiscard` | 처음 사용 시 볼륨 초기화 방법 |
| `wipeMethod` | `none`, `dd`, `deleteFiles`, `blkdiscard`, `blkdiscardWithHeaderCleanup`, `headerCleanup` | 파드 삭제 전 볼륨 초기화 방법 |
| `cascadeDelete` | `true` / `false` | 파드 삭제 시 PVC도 삭제할지 여부 |

### blockVolumePolicy

`volumeMode: Block`인 PVC 볼륨에 적용됩니다.

`filesystemVolumePolicy`와 동일한 필드를 사용합니다. 블록 디바이스에는 `initMethod: blkdiscard`(빠른 하드웨어 가속)와 `wipeMethod: blkdiscardWithHeaderCleanup`을 권장합니다.

### localStorageClasses

"로컬" 스토리지로 인식할 StorageClass 이름 목록 (예: `local-path`, `openebs-hostpath`). 로컬 PVC는 파드 재시작 전에 삭제되어 클린 상태를 보장합니다.

```yaml
storage:
  localStorageClasses:
    - local-path
    - openebs-hostpath
  deleteLocalStorageOnRestart: true
  cleanupThreads: 2
```

자세한 내용은 아래 [로컬 스토리지](#로컬-스토리지) 섹션을 참조하세요.

---

## 볼륨 타입

### PVC — Filesystem (기본)

블록 기반 파일시스템. 데이터, 로그, WAL 파일에 적합합니다.

```yaml
volumes:
  - name: data-vol
    source:
      persistentVolume:
        storageClass: standard
        size: 50Gi
        volumeMode: Filesystem   # 기본값
    aerospike:
      path: /opt/aerospike/data
```

### PVC — Block Device

파일시스템이 없는 원시 블록 볼륨. 스토리지에 직접 접근하는 Aerospike device 모드 스토리지 엔진에 사용합니다.

```yaml
volumes:
  - name: sindex-vol
    source:
      persistentVolume:
        storageClass: fast-ssd
        size: 20Gi
        volumeMode: Block
    aerospike:
      path: /opt/aerospike/sindex
```

### EmptyDir

임시 파드 내 스토리지. Aerospike 컨테이너와 사이드카(예: exporter) 간 데이터 공유에 유용합니다. 파드 재시작 시 데이터가 소실됩니다.

```yaml
volumes:
  - name: shared-data
    source:
      emptyDir: {}
    aerospike:
      path: /opt/aerospike/shared
      subPath: "aerospike-data"
    sidecars:
      - containerName: aerospike-prometheus-exporter
        path: /shared
        readOnly: true
```

### HostPath

호스트 노드의 디렉토리를 마운트합니다. 개발/테스트 전용 — 데이터가 특정 노드에 종속됩니다.

```yaml
volumes:
  - name: host-logs
    source:
      hostPath:
        path: /var/log/aerospike
        type: DirectoryOrCreate
    aerospike:
      path: /opt/aerospike/logs
```

### CSI 볼륨 (mountPropagation)

`HostToContainer` 전파가 필요한 CSI 드라이버(예: FUSE 기반 스토리지)에 사용합니다.

```yaml
volumes:
  - name: csi-data
    source:
      persistentVolume:
        storageClass: csi-driver
        size: 100Gi
    aerospike:
      path: /opt/aerospike/csi
      mountPropagation: HostToContainer
```

---

## 볼륨별 오버라이드

각 볼륨은 `initMethod`, `wipeMethod`, `cascadeDelete`로 글로벌 정책을 오버라이드할 수 있습니다:

```yaml
storage:
  filesystemVolumePolicy:
    initMethod: deleteFiles   # 글로벌 기본값
  volumes:
    - name: data-vol
      source:
        persistentVolume:
          storageClass: standard
          size: 50Gi
      aerospike:
        path: /opt/aerospike/data
      initMethod: dd          # 오버라이드: 이 볼륨에만 dd 사용
```

우선순위: **볼륨별** > **글로벌 정책** > **오퍼레이터 기본값**

---

## PVC 메타데이터

PVC에 커스텀 레이블과 어노테이션을 추가할 수 있습니다 — 백업 도구, 스토리지 정책, 비용 추적에 유용합니다:

```yaml
volumes:
  - name: data-vol
    source:
      persistentVolume:
        storageClass: standard
        size: 50Gi
        metadata:
          labels:
            app.kubernetes.io/data-tier: "aerospike"
            backup-policy: "daily"
          annotations:
            volume.kubernetes.io/storage-provisioner: "ebs.csi.aws.com"
    aerospike:
      path: /opt/aerospike/data
```

---

## 완전한 예제

다음 예제(`config/samples/aerospike-cluster-storage-advanced.yaml`)는 모든 볼륨 타입과 정책 옵션을 단일 클러스터에서 보여줍니다:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-storage-advanced
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1

  storage:
    # 글로벌 볼륨 정책: 볼륨별 오버라이드가 없으면 모든 영구 볼륨에 적용
    filesystemVolumePolicy:
      initMethod: deleteFiles
      wipeMethod: deleteFiles
      cascadeDelete: true
    blockVolumePolicy:
      initMethod: blkdiscard
      wipeMethod: blkdiscardWithHeaderCleanup

    # 로컬 스토리지 인식: 파드 재시작 전 로컬 PVC 삭제
    localStorageClasses:
      - local-path
      - openebs-hostpath
    deleteLocalStorageOnRestart: true

    cleanupThreads: 2

    volumes:
      # 커스텀 메타데이터가 있는 PVC (PVC 자체의 라벨 및 어노테이션)
      - name: data-vol
        source:
          persistentVolume:
            storageClass: standard
            size: 50Gi
            volumeMode: Filesystem
            metadata:
              labels:
                app.kubernetes.io/data-tier: "aerospike"
                backup-policy: "daily"
              annotations:
                volume.kubernetes.io/storage-provisioner: "ebs.csi.aws.com"
        aerospike:
          path: /opt/aerospike/data
        # 볼륨별 오버라이드: 글로벌 정책의 deleteFiles 대신 dd 사용
        initMethod: dd

      # 블록 디바이스 볼륨 (blockVolumePolicy 기본값 사용)
      - name: sindex-vol
        source:
          persistentVolume:
            storageClass: fast-ssd
            size: 20Gi
            volumeMode: Block
        aerospike:
          path: /opt/aerospike/sindex
        # initMethod 미설정 → blockVolumePolicy.initMethod (blkdiscard)로 폴백

      # 고급 마운트 옵션이 있는 볼륨
      - name: shared-data
        source:
          emptyDir: {}
        aerospike:
          path: /opt/aerospike/shared
          readOnly: false
          subPath: "aerospike-data"
        sidecars:
          - containerName: aerospike-prometheus-exporter
            path: /shared
            readOnly: true

      # HostPath 볼륨 (개발/테스트 전용)
      - name: host-logs
        source:
          hostPath:
            path: /var/log/aerospike
            type: DirectoryOrCreate
        aerospike:
          path: /opt/aerospike/logs

      # mountPropagation이 있는 볼륨
      - name: csi-data
        source:
          persistentVolume:
            storageClass: csi-driver
            size: 100Gi
        aerospike:
          path: /opt/aerospike/csi
          mountPropagation: HostToContainer

      # 로컬 스토리지 볼륨
      - name: local-data
        source:
          persistentVolume:
            storageClass: local-path
            size: 10Gi
        aerospike:
          path: /opt/aerospike/local
        initMethod: deleteFiles
        wipeMethod: deleteFiles

  aerospikeConfig:
    service:
      cluster-name: storage-advanced-demo
    namespaces:
      - name: test
        indexes-memory-budget: 1073741824
        replication-factor: 2
        storage-engine:
          type: device
          devices:
            - /opt/aerospike/data/test.dat
          filesize: 4294967296
          data-in-memory: true
```

적용:

```bash
kubectl apply -f config/samples/aerospike-cluster-storage-advanced.yaml
```

---

## 로컬 스토리지

로컬 영구 볼륨(로컬 PV)은 노드에 바인딩된 스토리지로 높은 I/O 성능을 제공하지만, 파드가 다른 노드로 마이그레이션되면 생존할 수 없습니다. 오퍼레이터는 로컬 스토리지를 올바르게 처리하기 위해 `localStorageClasses`와 `deleteLocalStorageOnRestart` 두 가지 필드를 제공합니다.

### 로컬 스토리지 사용 시기

- **고성능 워크로드** -- 로컬 NVMe 또는 SSD 스토리지가 가장 낮은 지연 시간을 제공
- **베어메탈 클러스터** -- 직접 연결된 스토리지가 있는 노드
- **비용 최적화** -- 데이터가 Aerospike 레벨에서 복제될 때 네트워크 연결 스토리지 오버헤드 회피

:::warning
로컬 PV는 특정 노드에 종속됩니다. 파드가 다른 노드로 재스케줄링되면 이전 PV를 다시 연결할 수 없습니다. `deleteLocalStorageOnRestart`가 활성화되면 오퍼레이터가 이를 자동으로 처리합니다.
:::

### 설정

#### `localStorageClasses`

오퍼레이터가 로컬 스토리지로 취급해야 하는 StorageClass 이름 목록입니다. 이러한 클래스로 프로비저닝된 PVC는 파드 재시작 시 특별한 처리를 받습니다.

일반적인 로컬 StorageClass 예시:
- `local-path` (Rancher Local Path Provisioner)
- `openebs-hostpath` (OpenEBS)
- `local-storage` (Kubernetes `local` 볼륨 플러그인)

#### `deleteLocalStorageOnRestart`

`true`로 설정하면, 콜드 재시작 시 파드를 삭제하기 **전에** 로컬 스토리지 클래스로 프로비저닝된 PVC를 삭제합니다. 이렇게 하면 파드가 재스케줄링되는 노드에서 PVC가 다시 프로비저닝됩니다.

이 설정이 없으면, 파드가 다른 노드로 재스케줄링될 경우 로컬 PV가 원래 노드에서만 사용 가능하기 때문에 `Pending` 상태에서 멈출 수 있습니다.

### 설정 예제

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-local-storage
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1

  storage:
    localStorageClasses:
      - local-path
      - openebs-hostpath
    deleteLocalStorageOnRestart: true

    filesystemVolumePolicy:
      initMethod: deleteFiles
      cascadeDelete: true

    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: local-path
            size: 50Gi
        aerospike:
          path: /opt/aerospike/data

  aerospikeConfig:
    namespaces:
      - name: data
        replication-factor: 2
        storage-engine:
          type: device
          file: /opt/aerospike/data/data.dat
          filesize: 42949672960
```

### 동작 원리

콜드 재시작(이미지 변경, 설정 변경, PodRestart 오퍼레이션) 시:

1. 오퍼레이터가 `localStorageClasses`에 나열된 스토리지 클래스로 프로비저닝된 PVC를 식별합니다
2. `deleteLocalStorageOnRestart`가 `true`이면 해당 PVC를 파드보다 **먼저** 삭제합니다
3. 파드가 삭제됩니다
4. Kubernetes가 파드를 스케줄링합니다 (다른 노드일 수 있음)
5. StorageClass 프로비저너가 대상 노드에 새 로컬 PV를 생성합니다
6. 파드가 새 볼륨으로 시작됩니다

:::info
로컬 PVC 삭제가 실패하면, 오퍼레이터는 `LocalPVCDeleteFailed` 경고 이벤트를 발행하고 파드 재시작을 계속합니다. PVC는 다음 리컨실레이션에서 정리됩니다.
:::

### 랙 설정과 로컬 스토리지

랙 인식 배포에서 로컬 스토리지를 사용할 때, `localStorageClasses`와 `rackLabel` 스케줄링을 결합하여 로컬 스토리지가 있는 노드에 파드가 배치되도록 합니다:

```yaml
spec:
  storage:
    localStorageClasses:
      - local-path
    deleteLocalStorageOnRestart: true
    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: local-path
            size: 50Gi
        aerospike:
          path: /opt/aerospike/data

  rackConfig:
    racks:
      - id: 1
        rackLabel: zone-a    # acko.io/rack=zone-a 레이블이 있고 로컬 스토리지가 있는 노드
      - id: 2
        rackLabel: zone-b
```

### 로컬 스토리지 vs 네트워크 연결 스토리지

| 측면 | 로컬 스토리지 | 네트워크 연결 (EBS, PD 등) |
|------|-------------|--------------------------|
| **지연 시간** | 가장 낮음 (직접 연결) | 높음 (네트워크 왕복) |
| **파드 마이그레이션** | PVC를 삭제하고 재프로비저닝 필요 | PVC가 자동으로 재연결 |
| **데이터 내구성** | 노드 수준에서만 | 노드 라이프사이클과 독립적 |
| **비용** | 낮음 (네트워크 스토리지 비용 없음) | 높음 (프로비저닝된 IOPS, 처리량) |
| **적합한 용도** | 고처리량, Aerospike 복제 데이터 | 단일 복제본 또는 상태 유지 워크로드 |

:::tip
로컬 스토리지 사용 시 Aerospike에서 항상 `replication-factor: 2` 이상으로 설정하여 노드 장애 시 데이터가 보존되도록 하세요. 손실된 로컬 PV의 데이터는 다른 노드의 복제본에서 복구됩니다.
:::

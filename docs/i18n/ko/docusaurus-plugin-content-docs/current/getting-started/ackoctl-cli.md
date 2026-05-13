---
sidebar_position: 4
title: ackoctl CLI
---

# ackoctl — 커맨드라인 인터페이스

[ackoctl](https://github.com/aerospike-ce-ecosystem/ackoctl)은 [클러스터 매니저 UI](./cluster-manager-ui.md)의 CLI 대응 도구입니다. 둘 다 동일한 `aerospike-cluster-manager` REST API(`/api/v1/*`)를 호출합니다 — UI에서 가능한 모든 작업을 터미널이나 CI 파이프라인에서도 수행할 수 있습니다.

Go로 작성되었으며 `kubectl` / `gh` 스타일의 명령 문법(`ackoctl <noun> <verb>`)을 따릅니다. Kubernetes나 Aerospike에 직접 접근하지 않고, 항상 cluster-manager의 UI Deployment를 통해 동작합니다.

---

## 설치

### Homebrew (macOS, Linux)

```bash
brew install aerospike-ce-ecosystem/tap/ackoctl
```

### Debian / Ubuntu

```bash
sudo install -d /etc/apt/keyrings
curl -fsSL https://aerospike-ce-ecosystem.github.io/ackoctl/key.gpg \
  | sudo gpg --dearmor -o /etc/apt/keyrings/ackoctl.gpg
echo "deb [signed-by=/etc/apt/keyrings/ackoctl.gpg] https://aerospike-ce-ecosystem.github.io/ackoctl/apt stable main" \
  | sudo tee /etc/apt/sources.list.d/ackoctl.list
sudo apt update && sudo apt install ackoctl
```

### RHEL / Fedora / Rocky / AlmaLinux

```bash
sudo curl -fsSL https://aerospike-ce-ecosystem.github.io/ackoctl/yum/ackoctl.repo \
  -o /etc/yum.repos.d/ackoctl.repo
sudo dnf install ackoctl
```

### POSIX 셸 일반 (패키지 매니저 없이)

```bash
curl -fsSL https://raw.githubusercontent.com/aerospike-ce-ecosystem/ackoctl/main/install.sh | sh
```

확인:

```bash
ackoctl version
```

버전 핀, 커스텀 설치 디렉터리, 소스 빌드 등은 [ackoctl 설치 가이드](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/docs/install.md)를 참고하세요.

---

## cluster-manager 접속 설정

`ackoctl`은 `~/.ackoctl/config.yaml`을 읽습니다 — `kubectl`과 동일한 멀티 컨텍스트 구조입니다.

cluster-manager가 동일 Kubernetes 클러스터 안에서 실행 중이라면(ACKO 차트 0.4.0 이후 기본 동작) `kubectl port-forward`로 노출시킵니다.

```bash
kubectl -n aerospike-operator port-forward \
  svc/acko-aerospike-ce-kubernetes-operator-ui 8000:8000
```

그 다음 `ackoctl`을 그곳에 연결합니다.

```bash
ackoctl config set-context kind-local \
  --server=http://localhost:8000/api \
  --workspace-id=default
ackoctl config use-context kind-local
ackoctl config view
```

원격 cluster-manager에 붙일 때는 port-forward 없이 공인 URL을 그대로 사용합니다.

```bash
ackoctl config set-context prod \
  --server=https://acm.example.com/api \
  --token="$(your-oidc-flow)" \
  --workspace-id=production
ackoctl config use-context prod
```

`ackoctl`에는 `login` 서브커맨드가 없습니다 — OIDC 토큰은 외부 도구(Keycloak CLI, 브라우저 device flow 등)로 얻어 `--token` 플래그나 `ACKOCTL_TOKEN` 환경 변수로 전달합니다.

우선 순위: **CLI 플래그 > 환경 변수 > 설정 파일**.

---

## 명령어 맵

```
ackoctl
├── version                                       바이너리 버전, 커밋, 빌드 시각
├── config       view | set-context | use-context | current-context | delete-context
├── connection   list | get | create | update | delete | health
├── cluster      info | configure-namespace
├── k8s cluster  list | get | reconcile
├── record       list | get | put | delete | query
├── set          list
├── query        exec
└── index        list | create | delete
```

모든 list/get 계열 명령은 `-o table|json|yaml`을 지원해 `jq`, `yq`, 또는 스크립트로 곧장 파이프할 수 있습니다.

---

## 주요 워크플로우

### ACKO가 관리하는 클러스터 조회

```bash
# 워크스페이스에 속한 모든 AerospikeCluster CR 조회
ackoctl k8s cluster list

# 특정 클러스터 상세
ackoctl k8s cluster get my-cluster -o yaml

# 강제 reconcile (CR을 직접 편집한 직후 등)
ackoctl k8s cluster reconcile my-cluster
```

### Aerospike 연결 등록 후 데이터 탐색

```bash
ackoctl connection create \
  --name local-aero \
  --host aerospike-node-1 --host aerospike-node-2 \
  --port 3000 \
  --namespace test

ackoctl connection health local-aero

ackoctl set list --connection local-aero --namespace test
ackoctl record list --connection local-aero --namespace test --set users --limit 20
```

### 임시 쿼리 실행

```bash
ackoctl query exec \
  --connection local-aero \
  --namespace test \
  --set users \
  --where 'age > 30' \
  --select name,email \
  -o json
```

### Secondary index 관리

```bash
ackoctl index list --connection local-aero --namespace test
ackoctl index create --connection local-aero --namespace test --set users \
  --name idx_age --bin age --type numeric
ackoctl index delete --connection local-aero --namespace test --name idx_age
```

---

## CI / 자동화에서 사용

상태를 변경하는 모든 명령은 `--workspace`(cluster-manager의 ACL 경계)를 존중합니다. 짧은 수명의 OIDC 토큰과 조합하면 어떤 CI 파이프라인에서도 ACKO reconcile을 구동할 수 있습니다.

```bash
export ACKOCTL_SERVER=https://acm.example.com/api
export ACKOCTL_TOKEN="${CI_OIDC_TOKEN}"
export ACKOCTL_WORKSPACE=production

ackoctl k8s cluster reconcile my-cluster
ackoctl k8s cluster get my-cluster -o json | jq -r '.status.phase'
```

cluster-manager의 에러는 구조화된 `{"detail": "..."}` 메시지와 종료 코드 1로 표면화되므로 `set -e`가 그대로 동작합니다.

---

## 더 보기

- 소스 리포지토리 — [aerospike-ce-ecosystem/ackoctl](https://github.com/aerospike-ce-ecosystem/ackoctl)
- 전체 명령 레퍼런스 — [`docs/usage.md`](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/docs/usage.md)
- 설치 옵션 — [`docs/install.md`](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/docs/install.md)
- 릴리스 노트 — [`CHANGELOG.md`](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/CHANGELOG.md)
- 클러스터 매니저 UI (웹 카운터파트) — [클러스터 매니저 UI 가이드](./cluster-manager-ui.md)

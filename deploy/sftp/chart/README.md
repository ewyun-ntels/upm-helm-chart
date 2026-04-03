# SFTP Helm Chart

이 Helm 차트는 Kubernetes 환경에서 SFTP 서버를 배포하기 위한 차트입니다. StatefulSet을 사용하여 안정적인 스토리지를 제공하고, MetalLB를 통해 외부 IP를 할당합니다.

helm upgrade --install upm-sftp . -n upm-sftp

## 🔄 최근 업데이트 (2026.03.23)
- **NFS 지원 추가**: 내장 NFS 서버 또는 외부 NFS 서버 사용 가능
- **스토리지 타입 선택**: PVC (local-path) 또는 NFS 마운트 선택 가능
- **MetalLB 지원**: loxilb에서 MetalLB로 LoadBalancer 변경
- **네임스페이스 통합**: NFS 서버를 차트와 동일한 네임스페이스에 배포
- **NFS Readiness Check**: InitContainer로 NFS 서버 준비 상태 자동 대기

## 주요 특징

- **StatefulSet 기반 배포**: 안정적인 네트워크 ID와 영구 스토리지 제공
- **유연한 스토리지**: PVC (각 Pod별 독립) 또는 NFS (공유) 선택 가능
- **내장 NFS 서버**: 차트에 NFS 서버 포함 (선택적)
- **MetalLB LoadBalancer**: Layer 2/BGP 모드로 외부 IP 할당
- **설정 가능한 SFTP 사용자**: ConfigMap을 통한 사용자 관리
- **재시작 시 데이터 유지**: 영구 스토리지를 통한 데이터 영속성 보장
- **멀티 컨테이너 지원**: Pod 내 여러 컨테이너가 볼륨을 공유하는 사이드카 패턴 지원 (선택적)

## 스토리지 아키텍처

### 옵션 1: PVC (기본값)
```
┌──────────┐    ┌──────────┐
│ sftp-0   │    │ sftp-1   │
│          │    │          │
│ ┌──────┐ │    │ ┌──────┐ │
│ │ PVC  │ │    │ │ PVC  │ │
│ │ 3Gi  │ │    │ │ 3Gi  │ │
│ └──────┘ │    │ └──────┘ │
└──────────┘    └──────────┘
   독립 스토리지    독립 스토리지
```

### 옵션 2: NFS (공유)
```
┌──────────┐    ┌──────────┐
│ sftp-0   │    │ sftp-1   │
│          │    │          │
│  NFS ────┼────┼──── NFS  │
└──────────┘    └──────────┘
        │            │
        └────┬───────┘
             │
      ┌──────▼─────┐
      │ NFS Server │
      │  /exports  │
      └────────────┘
       공유 스토리지
```

## 배포 방법

### A. PVC 모드로 배포 (기본)
```bash
# 각 Pod가 독립적인 PVC 사용
helm install sftp .
```

### B. 내장 NFS 서버와 함께 배포 (권장)
```bash
# Step 1: NFS 서버와 SFTP를 함께 설치
helm install sftp . \
  --set nfs.enabled=true \
  --set persistence.type=nfs

# NFS 서버가 먼저 준비되고 SFTP Pod가 자동으로 대기합니다
# InitContainer가 NFS 서버 readiness를 체크합니다
```

### C. 외부 NFS 서버 사용
```bash
helm install sftp . \
  --set persistence.type=nfs \
  --set persistence.nfs.server=192.168.1.100 \
  --set persistence.nfs.path=/exports/sftp
```

### D. 단계별 배포 (NFS 서버 준비 확인 필요 시)
```bash
# Step 1: NFS 서버만 먼저 배포
helm install sftp . --set nfs.enabled=true --set replicaCount=0

# Step 2: NFS 서버 준비 확인
kubectl wait --for=condition=ready pod -l app=nfs-server --timeout=120s

# Step 3: SFTP 활성화
helm upgrade sftp . \
  --set nfs.enabled=true \
  --set replicaCount=2 \
  --set persistence.type=nfs
```

**💡 참고**: NFS 모드 사용 시 InitContainer가 자동으로 NFS 서버가 준비될 때까지 대기하므로 B 방법이 가장 간단합니다.

## 주요 설정

이 차트는 다음 Kubernetes 리소스를 생성합니다:

### 1. StatefulSet
- **목적**: SFTP 서버 Pod를 안정적으로 관리
- **특징**:
  - 안정적인 네트워크 ID (sftp-0, sftp-1, ...)
  - 순차적인 배포 및 종료
  - 영구 스토리지 자동 마운트

### 2. Service (LoadBalancer)
- **목적**: 외부에서 SFTP 서버에 접근 가능하도록 설정
- **특징**:
  - loxilb incluster mode에서 onearm 방식으로 외부 IP 할당
  - Port 22 (SSH/SFTP) 노출
  - onearm 모드: Pod IP를 직접 사용하여 NAT 오버헤드 최소화

### 3. StorageClass
- **목적**: 동적 볼륨 프로비저닝을 위한 스토리지 클래스
- **특징**:
  - ReclaimPolicy: Retain (삭제 시 데이터 보존)
  - VolumeBindingMode: WaitForFirstConsumer
  - AllowVolumeExpansion: true (볼륨 확장 가능)

### 4. PersistentVolumeClaim (via VolumeClaimTemplates)
- **목적**: 각 Pod에 영구 스토리지 제공
- **특징**:
  - StatefulSet의 volumeClaimTemplates를 통해 자동 생성
  - Pod 재시작 시에도 동일한 PVC 재사용

### 5. ConfigMap
- **목적**: SFTP 사용자 정보 관리
- **형식**: `username:password:uid:gid:directory`

## 설치 방법

### 전제 조건

1. Kubernetes 클러스터 (v1.19+)
2. Helm 3.x
3. loxilb 설치 및 구성 (incluster mode)
4. StorageClass provisioner 설정 (환경에 맞게)

**loxilb 모드 참고**:
- **incluster mode + onearm**: Pod IP를 직접 사용, NAT 오버헤드 최소화 (권장)
- **external mode + fullnat**: 외부 loxilb 사용 시 SNAT/DNAT 적용

### 기본 설치

```bash
# 차트 디렉토리로 이동
cd /home/bigwo/nTels/07.UPM/sftp/chart

# 기본 설정으로 설치
helm install sftp-server .
```

### 커스텀 설정으로 설치

```bash
# values.yaml 파일 수정 후 설치
helm install sftp-server . -f values.yaml

# 또는 명령줄에서 값 오버라이드
helm install sftp-server . \
  --set service.annotations."loxilb\.io/staticIP"="192.168.1.100" \
  --set persistence.size=20Gi \
  --set replicaCount=2

# 멀티 컨테이너와 함께 설치 (커스텀 values 파일 사용 권장)
helm install sftp-server . -f custom-values.yaml
```

### 특정 네임스페이스에 설치

```bash
kubectl create namespace sftp
helm install sftp-server . -n sftp
```

## 설정 옵션 (values.yaml)

### 이미지 설정

```yaml
image:
  repository: atmoz/sftp  # SFTP 서버 이미지
  pullPolicy: IfNotPresent
  tag: "latest"
```

### Service 설정 (loxilb)

```yaml
service:
  type: LoadBalancer
  port: 22
  targetPort: 22
  loadBalancerClass: loxilb.io/loxilb           # loxilb LoadBalancer 클래스
  allocateLoadBalancerNodePorts: false         # NodePort 할당 비활성화 (loxilb onearm 모드에서 권장)
  annotations:
    loxilb.io/liveness: "yes"                 # loxilb 헬스체크 활성화
    loxilb.io/lbmode: "onearm"                # loxilb 로드밸런싱 모드 (incluster mode)
    loxilb.io/staticIP: "192.168.15.77"       # 원하는 외부 IP 지정
  externalTrafficPolicy: Cluster               # 트래픽 정책
```

**주요 설정 설명**:
- `loadBalancerClass`: loxilb LoadBalancer 사용 지정
- `allocateLoadBalancerNodePorts: false`: loxilb onearm 모드에서 NodePort를 우회하여 Pod로 직접 라우팅하므로 성능 향상
- `loxilb.io/liveness`: loxilb에서 헬스체크 활성화
- `loxilb.io/lbmode: "onearm"`: incluster mode에서 NAT 오버헤드 최소화

### StorageClass 설정

**⚠️ 중요: 환경별 Storage 설정**

#### 개발/테스트 환경 (K3s/Kind)
```yaml
storageClass:
  enabled: false  # 기존 local-path storage class 사용
  
persistence:
  enabled: true
  storageClassName: local-path  # K3s 기본 storage class
  accessMode: ReadWriteOnce
  size: 3Gi
  mountPath: /data
```

#### 운영 환경 (클라우드/온프레미스)
```yaml
storageClass:
  enabled: true
  name: sftp-storage
  # 환경별 Provisioner 선택 (아래 중 하나)
  provisioner: kubernetes.io/aws-ebs        # AWS EKS
  # provisioner: kubernetes.io/gce-pd       # Google GKE  
  # provisioner: kubernetes.io/azure-disk   # Azure AKS
  # provisioner: pd.csi.storage.gke.io      # GKE CSI Driver
  # provisioner: ebs.csi.aws.com           # AWS EBS CSI Driver
  # provisioner: disk.csi.azure.com        # Azure Disk CSI Driver
  # provisioner: kubernetes.io/vsphere-volume  # vSphere
  # provisioner: ceph.rook.io/block         # Rook-Ceph
  # provisioner: longhorn-system            # Longhorn
  reclaimPolicy: Retain
  volumeBindingMode: WaitForFirstConsumer
  allowVolumeExpansion: true
  
persistence:
  enabled: true
  storageClassName: sftp-storage
  accessMode: ReadWriteOnce  
  size: 10Gi                    # 운영환경 권장 최소 크기
  mountPath: /data              # Pod 내 마운트 경로
```

#### 주요 환경별 Provisioner

| 환경 | Provisioner | 특징 |
|------|-------------|------|
| **AWS EKS** | `ebs.csi.aws.com` | CSI Driver 권장, GP3/IO2 볼륨 |
| **Google GKE** | `pd.csi.storage.gke.io` | CSI Driver, SSD/HDD 지원 |
| **Azure AKS** | `disk.csi.azure.com` | Managed Disk, Premium/Standard |
| **vSphere** | `kubernetes.io/vsphere-volume` | VMDK 볼륨 |
| **Rook-Ceph** | `ceph.rook.io/block` | 고가용성 분산 스토리지 |
| **Longhorn** | `longhorn` | Rancher 분산 스토리지 |
| **K3s/개발** | `rancher.io/local-path` | 로컬 호스트 경로 (운영 X) |

**⚠️ local-path 사용 제한사항:**
- 개발/테스트 환경에서만 사용
- 노드 장애 시 데이터 유실 위험
- 운영환경에서는 반드시 분산/복제 스토리지 사용

## 데이터 저장 방식 (Data Storage Pattern)

### 📁 표준 디렉토리 구조

이 차트는 SFTP 사용자가 로그인 직후 **`/data`에서 바로 작업**할 수 있도록 구성합니다.
실제 쓰기 디렉터리는 `/home/<user>/data`이며, chroot 내부의 `/data`는 같은 위치를 가리킵니다:

```
/home/                    # SFTP chroot 경로
└── sftpuser/
    └── data/             # 사용자 실제 쓰기 경로

/data -> /home/sftpuser/data
```

**SFTP 사용자가 보는 구조 예시:**
```
/data
└── gochk.0.json
└── grafana.crt

/home/sftpuser/data
└── gochk.0.json
└── grafana.crt
```

**관계 요약:**
- `/data`와 `/home/sftpuser/data`는 같은 사용자 저장소를 가리킴
- SFTP 사용자는 로그인 후 `/data`에서 바로 작업 가능

### 🔐 권한 및 실행 사용자

**접근 보장 방식:**
- 컨테이너는 root 권한으로 실행 (PV 경로 접근 보장)
- SFTP 사용자 홈: `/home/<user>` (chroot)
- SFTP 사용자 쓰기 디렉터리: `/home/<user>/data` (PVC subPath)

### 🔄 데이터 영속성 보장

**Pod 재시작 시 데이터 유지:**
- ✅ SFTP 사용자가 `data/` 폴더에 업로드한 파일 → **영구 보존** (PV에 저장)
- ❌ 홈 디렉터리 루트나 다른 위치에 저장된 파일 → **재시작 시 삭제**

**설정 요약:**
```yaml
persistence:
  mountPath: /data        # PV 마운트 위치
  homeMount:
    enabled: true
    mountPath: /home/sftpuser/data
    subPath: sftpuser
  
sftpUsers:
  - "sftpuser:password:1001:1001:data"  # /home/<user>/data 생성 및 쓰기 디렉터리
```

**사용자 경험:**
- SFTP 연결 후 `/data`에서 바로 파일 작업
- 운영자는 `/home/<user>/data`에서 동일 데이터 확인 가능
- 업로드/다운로드가 곧바로 PV에 저장됨

### SFTP 사용자 설정

```yaml
sftpUsers:
  - "user1:pass1:1001:1001:data"
  - "user2:pass2:1002:1002:data" 
  # 형식: username:password:uid:gid:dir
  # dir은 /home/<user>/ 하위에 생성되는 디렉터리 (쓰기 위치)
```

**사용자별 하위 디렉토리 사용 시:**
```yaml
sftpUsers:
  - "user1:pass1:1001:1001:data"  # /home/user1/data 생성
  - "user2:pass2:1002:1002:data"  # /home/user2/data 생성
```

### 멀티 컨테이너 설정 (사이드카 패턴)

StatefulSet Pod 내에 추가 컨테이너를 실행할 수 있습니다. 모든 컨테이너는 동일한 PV를 공유합니다.

```yaml
additionalContainers:
  # 로그 수집 사이드카 예시
  - name: log-collector
    image: busybox:latest
    command: ["/bin/sh", "-c"]
    args:
      - |
        while true; do
          echo "Collecting logs at $(date)"
          sleep 300
        done
    volumeMounts:
    - name: data
      mountPath: /logs
      subPath: logs
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 50m
        memory: 64Mi
  
  # SFTP 파일을 HTTP로 제공하는 nginx 예시
  - name: nginx
    image: nginx:alpine
    ports:
    - containerPort: 80
      name: http
    volumeMounts:
    - name: data
      mountPath: /usr/share/nginx/html
      readOnly: true
    resources:
      limits:
        cpu: 200m
        memory: 256Mi
      requests:
        cpu: 100m
        memory: 128Mi
```

**멀티 컨테이너 사용 사례**:
- 로그 수집/모니터링 사이드카
- 파일 동기화 컨테이너
- HTTP/HTTPS 파일 서빙 (nginx, apache 등)
- 파일 처리/변환 워커
- 백업 에이전트

**PV 공유 방식**:
- 모든 컨테이너가 동일한 `data` 볼륨을 마운트할 수 있습니다
- `subPath`를 사용하여 각 컨테이너가 다른 디렉토리를 사용하도록 분리 가능
- 기본적으로 SFTP 컨테이너는 `/data`에 마운트됩니다

### 리소스 제한

```yaml
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

## 사용 예시

### 1. SFTP 클라이언트로 접속

```bash
# 외부 IP 확인
kubectl get svc sftp-server-sftp

# SFTP 접속
sftp -P 22 sftpuser@<EXTERNAL-IP>
```

### 2. 파일 업로드/다운로드

```bash
# SFTP 접속 후
sftp> put local-file.txt       # 파일 업로드
sftp> get remote-file.txt      # 파일 다운로드
sftp> ls                        # 파일 목록 확인
```

### 3. PVC 상태 확인

```bash
# PVC 목록 확인
kubectl get pvc

# 특정 PVC 상세 정보
kubectl describe pvc data-sftp-server-sftp-0
```

### 4. StatefulSet 확인

```bash
# StatefulSet 상태
kubectl get statefulset

# Pod 상태
kubectl get pods

# Pod 로그 확인 (SFTP 컨테이너)
kubectl logs sftp-server-sftp-0 -c sftp

# 특정 사이드카 컨테이너 로그 확인
kubectl logs sftp-server-sftp-0 -c nginx

# Pod 내 모든 컨테이너 확인
kubectl get pod sftp-server-sftp-0 -o jsonpath='{.spec.containers[*].name}'

# 특정 컨테이너에 접속
kubectl exec -it sftp-server-sftp-0 -c sftp -- /bin/bash
```

### 5. 멀티 컨테이너 PV 공유 확인

```bash
# SFTP 컨테이너에서 파일 생성
kubectl exec sftp-server-sftp-0 -c sftp -- touch /home/sftpuser/data/test-file.txt

# 다른 컨테이너에서 동일 파일 확인 (nginx 예시)
kubectl exec sftp-server-sftp-0 -c nginx -- ls -la /usr/share/nginx/html/

# 볼륨 마운트 정보 확인
kubectl describe pod sftp-server-sftp-0 | grep -A 10 "Mounts:"
```

## 업그레이드

```bash
# values.yaml 수정 후
helm upgrade sftp-server . -f values.yaml

# 특정 값만 변경
helm upgrade sftp-server . \
  --set persistence.size=20Gi
```

## 제거

```bash
# 차트 삭제
helm uninstall sftp-server

# PVC는 ReclaimPolicy가 Retain이므로 수동 삭제 필요
kubectl delete pvc data-sftp-server-sftp-0
```

**주의**: PVC를 삭제하면 데이터가 영구적으로 손실될 수 있습니다.

## 트러블슈팅

### 1. Service에 External IP가 할당되지 않는 경우

```bash
# loxilb가 정상 동작하는지 확인
kubectl get pods -n kube-system | grep loxilb

# Service 이벤트 확인
kubectl describe svc sftp-server-sftp

# loxilb 어노테이션 확인
kubectl get svc sftp-server-sftp -o yaml | grep annotations -A 5
```

### 2. PVC가 Pending 상태인 경우

#### 일반적인 원인과 해결방법

**A. StorageClass 문제**
```bash
# StorageClass 확인
kubectl get storageclass

# 사용 중인 StorageClass가 없거나 no-provisioner인 경우
kubectl describe pvc data-sftp-0
```

**해결방법:**
```yaml
# values.yaml에서 기존 StorageClass 사용
storageClass:
  enabled: false  # 커스텀 StorageClass 생성 안 함

persistence:
  storageClassName: "local-path"  # 또는 환경의 기본 StorageClass 이름
```

**B. no-provisioner 오류 (수동 PV 필요)**
```
Events:
  Warning  ProvisioningFailed  persistentvolume-controller  
          storageclass "sftp-storage" not found
  Warning  FailedScheduling   default-scheduler  
          0/3 nodes are available: 3 node(s) didn't find available persistent volumes to bind
```

**해결방법 1: 기존 StorageClass 사용**
```bash
# 사용 가능한 StorageClass 확인 
kubectl get storageclass

# values.yaml 수정
helm upgrade sftp ./chart -n upm-sftp \
  --set storageClass.enabled=false \
  --set persistence.storageClassName=local-path
```

**해결방법 2: 수동 PV 생성 (no-provisioner 사용 시)**
```yaml
# manual-pv.yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: sftp-pv-0
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteOnce
  storageClassName: sftp-storage
  hostPath:
    path: /mnt/sftp-data
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: In
          values:
          - worker-01  # 특정 노드 지정
```

**C. Provisioner 누락**
```bash
# Provisioner Pod 확인
kubectl get pods -A | grep -i provisioner

# CSI Driver 확인 (클라우드 환경)
kubectl get csidrivers
```

### 3. Pod가 시작되지 않는 경우

```bash
# Pod 이벤트 확인
kubectl describe pod sftp-server-sftp-0

# Pod 로그 확인
kubectl logs sftp-server-sftp-0

# 리소스 부족 확인
kubectl top nodes
```

### 4. SFTP 접속이 안 되는 경우

```bash
# Service 확인
kubectl get svc

# Pod 상태 확인
kubectl get pods

# Pod 내부에서 SFTP 프로세스 확인
kubectl exec sftp-server-sftp-0 -- ps aux | grep sshd

# 포트 확인
kubectl exec sftp-server-sftp-0 -- netstat -tlnp
```

## Storage 구성 가이드

### 🔧 현재 적용된 변경사항 (2026.02.05)

**개발/테스트 환경 최적화:** 
- `storageClass.enabled: false` - 기존 K3s의 `local-path` StorageClass 재사용
- `persistence.storageClassName: local-path` - 동적 프로비저닝 활성화  
- `kubernetes.io/no-provisioner` 제거 - 수동 PV 생성 불필요

**변경 이유:**
- ❌ **이전**: `no-provisioner`로 인한 Pod 스케줄링 실패
- ✅ **현재**: K3s 기본 `local-path` 사용으로 자동 볼륨 생성

### 🏗️ 운영 환경 Storage 권장사항

#### 1. 고가용성 스토리지 (권장)
```yaml
# 클라우드 환경
storageClass:
  enabled: true
  name: sftp-ha-storage
  provisioner: ebs.csi.aws.com          # AWS
  parameters:
    type: gp3
    fsType: ext4
    encrypted: "true"
  reclaimPolicy: Retain
  allowVolumeExpansion: true

persistence:
  size: 50Gi                           # 운영환경 권장
  storageClassName: sftp-ha-storage
```

#### 2. 온프레미스 분산 스토리지
```yaml
# Rook-Ceph 예시
storageClass:
  enabled: true
  name: sftp-ceph-storage
  provisioner: ceph.rook.io/block
  parameters:
    clusterID: rook-ceph
    pool: replicapool
    imageFormat: "2"
    imageFeatures: layering
  reclaimPolicy: Retain
  allowVolumeExpansion: true
```

#### 3. 환경별 볼륨 크기 가이드

| 환경 | 권장 크기 | 용도 |
|------|-----------|------|
| **개발/테스트** | 1-5Gi | 기능 검증 |
| **스테이징** | 10-20Gi | 성능 테스트 |
| **운영 (소규모)** | 50-100Gi | 일반 파일 전송 |
| **운영 (대규모)** | 200Gi+ | 대용량 데이터 처리 |

### ⚠️ 주의사항

**local-path 사용 제한:**
- ✅ K3s/Kind 개발환경에서만 사용
- ❌ 운영환경에서는 **절대 사용 금지**
- ❌ 노드 장애 시 데이터 완전 유실
- ❌ Pod 재스케줄링 시 다른 노드의 데이터 접근 불가

**운영환경 필수사항:**
- 📊 **모니터링**: 볼륨 사용률 모니터링 (80% 알림)
- 🔄 **백업**: 정기 백업/스냅샷 정책
- 🚀 **확장성**: `allowVolumeExpansion: true` 설정
- 🔒 **암호화**: 클라우드 볼륨 암호화 활성화

### 🔄 환경 마이그레이션 예시

**개발 → 운영 환경 이전 시:**
```bash
# 1. 현재 데이터 백업
kubectl exec sftp-0 -n upm-sftp -- tar -czf /tmp/backup.tar.gz /data

# 2. 운영용 values.yaml로 재배포
helm upgrade sftp ./chart -n upm-sftp -f production-values.yaml

# 3. 데이터 복원
kubectl cp sftp-0:/tmp/backup.tar.gz ./backup.tar.gz -n upm-sftp
kubectl exec sftp-0 -n upm-sftp -- tar -xzf /tmp/backup.tar.gz -C /
```

## 보안 고려사항

1. **패스워드 관리**: 
   - values.yaml의 패스워드를 Kubernetes Secret으로 관리하는 것을 권장
   - 또는 외부 Secrets 관리 도구 사용 (Vault, Sealed Secrets 등)

2. **네트워크 정책**:
   - NetworkPolicy를 사용하여 특정 IP 대역만 접근 허용

3. **SSH 키 인증**:
   - 패스워드 대신 SSH 키 기반 인증 사용 권장

4. **감사 로깅**:
   - SFTP 접속 및 파일 전송 로그 모니터링

## 참고 자료

- [Kubernetes StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
- [Kubernetes Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [loxilb Documentation](https://github.com/loxilb-io/loxilb)
- [atmoz/sftp Docker Image](https://github.com/atmoz/sftp)

## 라이선스

이 차트는 MIT 라이선스 하에 배포됩니다.

## 지원

문제가 발생하거나 기능 요청이 있으시면 이슈를 등록해주세요.


## Service 참고
bigwo@ewyun:~$ oc exec -it -n upm-loadbalancing loxilb-lb-547dfdb88f-v2lzm -- loxicmd get lb -o wide
|    EXT IP     | SEC IPS | SOURCES | HOST | PORT | PROTO |              NAME               | MARK | SEL |  MODE  |   ENDPOINT   | EPORT | WEIGHT |  STATE   | COUNTERS  |
|---------------|---------|---------|------|------|-------|---------------------------------|------|-----|--------|--------------|-------|--------|----------|-----------|
| 192.168.15.77 |         |         |      |   22 | tcp   | upm-sftp_sftp:llb-inst0         |    0 | rr  | onearm | 172.16.4.201 |     0 |      1 | active   | 158:23781 |
|               |         |         |      |      |       |                                 |      |     |        | 172.16.4.202 |     0 |      1 | active   | 34:1360   |
|               |         |         |      |      |       |                                 |      |     |        | 172.16.4.203 |     0 |      1 | active   | 89:14658  |
| 192.168.15.78 |         |         |      |   80 | tcp   | default_tcp-lb-onearm:llb-inst0 |    0 | rr  | onearm | 172.16.4.201 | 32394 |      1 | active   | 0:0       |
|               |         |         |      |      |       |                                 |      |     |        | 172.16.4.202 | 32394 |      1 | inactive | 0:0       |
|               |         |         |      |      |       |                                 |      |     |        | 172.16.4.203 | 32394 |      1 | active   | 0:0       |

EPORT(NodePort)가 없으면 일단 안됨.... 추가 확인 필요

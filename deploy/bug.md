# tcp-bridge Multus/ipvlan 네트워크 트러블슈팅

날짜: 2026-04-03

---

## 문제 1: ipvlan L3 모드에서 Pod 샌드박스 생성 실패

### 증상
```
failed to add route '{0.0.0.0 00000000} via 10.255.254.1 dev net1': network is unreachable
```
Pod가 `FailedCreatePodSandBox` 상태로 기동 불가.

### 원인
1. **ipvlan mode: l3** — L3 모드는 ARP 없이 순수 IP 라우팅만 수행
2. **Pod annotation에서 IP를 `/32`로 할당** — `net1` 인터페이스 입장에서 gateway `10.255.254.1`이 같은 서브넷에 없음
3. 결과적으로 `0.0.0.0/0 via 10.255.254.1` 라우트 추가 시 "network is unreachable" 발생

### 수정
| 파일 | 변경 내용 |
|------|-----------|
| `values.yaml` | `mode: "l3"` → `mode: "l2"` |
| `values.yaml` | 불필요한 `10.255.254.1/32 via 0.0.0.0` 라우트 제거 |
| `deployment-0.yaml` | IP prefix `/32` → `/24` |
| `deployment-1.yaml` | IP prefix `/32` → `/24` |

`/24`로 변경하면 `10.255.254.0/24` 대역이 `net1`에 directly connected로 인식되어 gateway가 도달 가능해짐.

---

## 문제 2: Liveness/Readiness Probe 실패 (no route to host / context deadline exceeded)

### 증상
```
Liveness probe failed: Get "http://10.244.x.x:8080/health": dial tcp: connect: no route to host
Readiness probe failed: Get "http://10.244.x.x:8080/health": context deadline exceeded
```
Pod는 기동되고 TCP 통신도 정상이나 health probe만 지속 실패 → CrashLoopBackOff.

### 원인
**비대칭 라우팅 (Asymmetric Routing)**

- 노드 IP 대역: `10.255.254.0/24` (worker-app01~03: .25~.27)
- multus 서브넷도 동일: `10.255.254.0/24`

kubelet이 pod에 health check를 보낼 때 source IP가 `10.255.254.26` (노드 IP).

Pod 내부 라우팅 테이블:
```
default via 169.254.1.1 dev eth0        ← Calico
10.255.254.0/24 dev net1 proto kernel   ← ipvlan (자동 생성)
192.168.15.0/24 via 10.255.254.1 dev net1
```

패킷 흐름:
- kubelet → pod: Calico veth (eth0) 경유 ✓
- pod → kubelet 응답: `10.255.254.26`이 `10.255.254.0/24` 매칭 → **net1(ipvlan)으로 잘못 나감** ✗

ipvlan을 통해 나간 응답은 kubelet에 도달하지 못해 probe timeout 발생.

### 수정
init container로 **policy routing** 설정:
- eth0 IP에서 발신하는 트래픽은 항상 eth0 경유하도록 별도 라우팅 테이블(table 100) 추가

```yaml
initContainers:
- name: setup-routing
  securityContext:
    runAsUser: 0
    capabilities:
      add: ["NET_ADMIN"]
  command:
  - /bin/sh
  - -c
  - |
    ETH0_IP=$(ip addr show eth0 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
    ip rule add from $ETH0_IP lookup 100
    ip route add default via 169.254.1.1 dev eth0 table 100
```

`deployment-0.yaml`, `deployment-1.yaml` 양쪽에 동일하게 적용.

---

## 최종 설정 구조

```
Pod
├── eth0 (Calico): 10.244.x.x/32
│   └── default route: 169.254.1.1 (Calico gateway)
│   └── policy rule: from 10.244.x.x → table 100 (eth0 강제)  ← init container
└── net1 (ipvlan l2): 10.255.254.10x/24
    └── route: 192.168.15.0/24 via 10.255.254.1 (TCP endpoint 전용)
```

- kubelet probe, NATS, cluster 내부 통신 → **eth0** (Calico)
- TCP endpoint(`192.168.15.201`) 연결 → **net1** (ipvlan, source IP 고정)

---

## 확인 명령어

### Pod 상태

```bash
# Pod 목록 및 상태
oc get pods -n upm-app -l app.kubernetes.io/name=tcp-bridge -o wide

# Pod 이벤트 (에러 원인 확인)
oc describe pod -n upm-app -l app.kubernetes.io/name=tcp-bridge

# Pod 로그
oc logs -n upm-app deploy/tcp-bridge-0 --tail=50
oc logs -n upm-app deploy/tcp-bridge-1 --tail=50

# init container 로그 (policy routing 적용 결과)
oc logs -n upm-app <pod-name> -c setup-routing
```

### Pod 내부 네트워크 (exec)

```bash
# 인터페이스 목록 및 IP 확인
oc exec -n upm-app <pod-name> -- ip addr

# 라우팅 테이블 확인 (기본)
oc exec -n upm-app <pod-name> -- ip route

# policy routing 룰 확인
oc exec -n upm-app <pod-name> -- ip rule

# 라우팅 테이블 100 확인 (init container가 설정한 eth0 전용 테이블)
oc exec -n upm-app <pod-name> -- ip route show table 100

# 특정 목적지로의 경로 확인
oc exec -n upm-app <pod-name> -- ip route get 10.255.254.26   # 노드 IP → net1이면 비대칭 라우팅 문제
oc exec -n upm-app <pod-name> -- ip route get 192.168.15.201  # TCP endpoint → net1이어야 정상
```

### 노드에서 Pod 네트워크 네임스페이스 직접 확인

```bash
# 노드 SSH 접속
sshpass -p 'pro00#uid' ssh root@10.255.254.26

# 컨테이너 PID 조회
crictl --runtime-endpoint unix:///run/containerd/containerd.sock ps | grep tcp-bridge
PID=$(crictl --runtime-endpoint unix:///run/containerd/containerd.sock inspect <container-id> \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["info"]["pid"])')

# Pod 네트워크 네임스페이스에서 직접 확인
nsenter -t $PID -n ip addr
nsenter -t $PID -n ip route
nsenter -t $PID -n ip rule
nsenter -t $PID -n ip route show table 100

# 노드에서 Pod IP로 직접 health check
curl -v --max-time 5 http://<pod-calico-ip>:8080/health

# 노드의 Pod IP 라우팅 경로 확인 (Calico veth 여부)
ip route get <pod-calico-ip>
```

### NAD (NetworkAttachmentDefinition) 확인

```bash
# NAD 목록
oc get network-attachment-definitions -n upm-app

# NAD 상세 (CNI 설정 확인)
oc get network-attachment-definitions tcp-bridge-net -n upm-app -o yaml

# Pod에 붙은 네트워크 인터페이스 상태
oc get pod <pod-name> -n upm-app -o jsonpath='{.metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}' | python3 -m json.tool
```

### Helm

```bash
# 릴리즈 상태 확인
helm list -n upm-app

# 현재 적용된 values 확인
helm get values tcp-bridge -n upm-app

# 업그레이드
helm upgrade tcp-bridge ./tcp-bridge/chart -n upm-app

# 렌더링 결과 미리보기 (실제 적용 전)
helm template tcp-bridge ./tcp-bridge/chart -n upm-app | grep -A 30 'initContainers'
```

---

## 참고: ipvlan vs macvlan

현재 설정은 `ipvlan l2` 모드 사용 중.

| 항목 | ipvlan (현재) | macvlan |
|------|--------------|---------|
| MAC 주소 | 호스트와 **공유** | pod마다 **고유 MAC 생성** |
| ARP | L2 모드에서 사용 | 사용 |
| mode 옵션 | l2 / l3 | bridge / private / vepa |
| 스위치 MAC 제한 | 영향 없음 | pod 수만큼 MAC 추가 필요 |

### macvlan으로 변경 시 설정

```yaml
type: "macvlan"
mode: "bridge"   # ipvlan l2 → macvlan bridge
```

나머지 ipam, routes, gateway 설정은 그대로 사용 가능.

### 주의사항

1. **init container policy routing 여전히 필요** — macvlan도 `net1`에 `10.255.254.0/24`가 붙으므로 노드 IP(`10.255.254.x`)로의 비대칭 라우팅 문제는 동일하게 발생
2. **스위치 MAC 제한 확인 필요** — macvlan은 pod마다 새 MAC이 생성되므로 물리 스위치에 port당 MAC 개수 제한 또는 MAC 필터링이 있으면 차단될 수 있음
3. **현재 ipvlan l2가 정상 동작 중**이므로 외부 장비에서 ARP로 pod MAC을 직접 인식해야 하는 특별한 이유가 없다면 변경 불필요

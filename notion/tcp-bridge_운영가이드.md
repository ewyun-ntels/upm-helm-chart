# tcp-bridge 운영 가이드

> Notion 위치: UPM Deploy > tcp-bridge > 운영 가이드

---

## 네트워크 구조

```
Pod
├── eth0 (Calico): 10.244.x.x/24
│   └── default route: 169.254.1.1 (Calico gateway)
│   └── kubelet probe, NATS, 클러스터 내부 통신
└── net1 (macvlan bridge): 10.255.254.10x/24
    └── route: 192.168.15.0/24 via 10.255.254.1
    └── TCP endpoint(192.168.15.x) 전용 외부 통신
```

---

## init container: setup-routing

노드 IP 대역(10.255.254.x)과 net1 대역이 겹쳐 비대칭 라우팅 발생 시 적용.
eth0 IP에서 발신하는 트래픽은 항상 eth0 경유하도록 policy routing 설정.

```yaml
initContainers:
- name: setup-routing
  image: busybox
  securityContext:
    runAsUser: 0
    capabilities:
      add: ["NET_ADMIN"]
  command:
  - /bin/sh
  - -c
  - |
    ETH0_IP=$(ip addr show eth0 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
    ETH0_GW=$(ip route show default dev eth0 | awk '/default/ {print $3; exit}')
    if [ -z "$ETH0_IP" ] || [ -z "$ETH0_GW" ]; then
      echo "failed to detect eth0 ip or gateway"
      exit 1
    fi
    ip rule add from $ETH0_IP lookup 100
    ip route add default via $ETH0_GW dev eth0 table 100
```

`deployment-0.yaml`, `deployment-1.yaml` 양쪽에 동일하게 적용.

> **제거 가능 조건**: 아래 체크리스트 참고. 노드 IP 대역과 net1 대역이 분리된 경우 불필요.

---

## ipvlan vs macvlan 비교

| 항목 | ipvlan (이전) | macvlan (현재) |
|---|---|---|
| MAC 주소 | 호스트와 공유 | Pod마다 고유 MAC 생성 |
| ARP | L2 모드에서 사용 | 사용 |
| mode | l2 / l3 | bridge / private / vepa |
| 스위치 MAC 제한 | 영향 없음 | Pod 수만큼 MAC 추가 필요 |

**macvlan 주의사항**: 물리 스위치에 port당 MAC 개수 제한 또는 MAC 필터링이 있으면 차단 가능

---

## 상태 확인 명령어

### Pod 상태

```bash
kubectl get pods -n upm-app -l app.kubernetes.io/name=tcp-bridge -o wide
kubectl describe pod -n upm-app -l app.kubernetes.io/name=tcp-bridge
kubectl logs -n upm-app deploy/tcp-bridge-0 --tail=50
kubectl logs -n upm-app deploy/tcp-bridge-1 --tail=50
```

### Pod 내부 네트워크

```bash
# 인터페이스 및 IP 확인
kubectl exec -n upm-app <pod-name> -- ip addr

# 라우팅 테이블
kubectl exec -n upm-app <pod-name> -- ip route

# policy routing 룰
kubectl exec -n upm-app <pod-name> -- ip rule

# eth0 전용 테이블 (init container 설정)
kubectl exec -n upm-app <pod-name> -- ip route show table 100

# 경로 확인
kubectl exec -n upm-app <pod-name> -- ip route get <node-ip>       # eth0이어야 정상
kubectl exec -n upm-app <pod-name> -- ip route get 192.168.15.201  # net1이어야 정상
```

### NAD (NetworkAttachmentDefinition)

```bash
kubectl get network-attachment-definitions -n upm-app
kubectl get network-attachment-definitions tcp-bridge-net -n upm-app -o yaml

# Pod에 붙은 네트워크 인터페이스 상태
kubectl get pod <pod-name> -n upm-app \
  -o jsonpath='{.metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}' \
  | python3 -m json.tool
```

### 노드에서 직접 확인

```bash
# crictl로 컨테이너 PID 조회
crictl --runtime-endpoint unix:///run/containerd/containerd.sock ps | grep tcp-bridge
PID=$(crictl --runtime-endpoint unix:///run/containerd/containerd.sock inspect <container-id> \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["info"]["pid"])')

# Pod 네트워크 네임스페이스에서 직접 확인
nsenter -t $PID -n ip addr
nsenter -t $PID -n ip route
nsenter -t $PID -n ip rule
nsenter -t $PID -n ip route show table 100

# 노드 → Pod health check
curl -v --max-time 5 http://<pod-calico-ip>:8080/health
```

### Helm

```bash
helm list -n upm-app
helm get values tcp-bridge -n upm-app
helm upgrade tcp-bridge ./tcp-bridge/chart -n upm-app

# 렌더링 미리보기
helm template tcp-bridge ./tcp-bridge/chart -n upm-app | grep -A 30 'initContainers'
```

---

## init container 제거 판단 체크리스트

아래 조건을 **모두** 만족할 때만 제거 검토

| 확인 항목 | 명령어 | 기대 결과 |
|---|---|---|
| 노드 IP 대역과 net1 대역이 겹치지 않음 | `kubectl get nodes -o wide` / `ip addr` | 대역 분리 확인 |
| kubelet probe 지속 성공 | `kubectl describe pod` | Liveness/Readiness 실패 없음 |
| 노드 IP 경로가 eth0으로 선택 | `ip route get <node-ip>` | `dev eth0` |
| TCP endpoint만 net1으로 라우팅 | `ip route get 192.168.15.201` | `dev net1` |
| 재배포 후에도 동일하게 정상 | `kubectl rollout restart` 후 확인 | 재현 없음 |

하나라도 불확실하면 init container 유지.

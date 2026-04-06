# Alerta Helm Chart

이 디렉터리에는 Alerta 배포용 Helm 차트와 환경별 values 파일이 있습니다.

## values 파일 용도

- `values.yaml`
  - 기본값 파일입니다.
  - 현재는 `upmdb-cluster-rw`로 직접 접속하는 `DATABASE_URL`이 포함되어 있습니다.
  - Service 기본값은 `NodePort` 이며 `30880` 포트를 사용합니다.
  - Alerta 기본 environment 설정으로 `ALLOWED_ENVIRONMENTS=Production,Development,RealTimeAlert,MetricAlert` 와 `DEFAULT_ENVIRONMENT=RealTimeAlert` 가 포함되어 있습니다.
  - 별도 `-f` 옵션 없이 설치하면 이 파일이 사용됩니다.

- `values-postgres-rw.yaml`
  - CloudNativePG의 RW 서비스(`upmdb-cluster-rw`)로 직접 접속할 때 사용하는 오버라이드 파일입니다.
  - 운영에서 기본값과 분리해 관리하고 싶을 때 사용합니다.

- `values-pgbouncer.yaml`
  - PgBouncer 서비스(`upmdb-pooler`)를 통해 접속할 때 사용하는 오버라이드 파일입니다.
  - 커넥션 수가 많거나 풀링이 필요한 환경에 적합합니다.

## 사용 예시

- 기본값(`values.yaml`)으로 설치

```bash
helm upgrade --install alerta ./alerta -n alerta --create-namespace
```

- RW 서비스 직접 접속 설정으로 설치

```bash
helm upgrade --install alerta ./alerta -n alerta --create-namespace -f alerta/values-postgres-rw.yaml
```

- PgBouncer 경유 설정으로 설치

```bash
helm upgrade --install alerta ./alerta -n alerta --create-namespace -f alerta/values-pgbouncer.yaml
```

## 주의

- `SECRET_KEY`, `ADMIN_KEY`, `DATABASE_URL` 비밀번호는 운영값으로 교체해서 사용하세요.
- Prometheus/Alertmanager가 보내는 alert label `environment` 값이 `ALLOWED_ENVIRONMENTS`에 없으면 Alerta에서 reject 됩니다.
- 현재 차트 기본값은 `Production`, `Development`, `RealTimeAlert`, `MetricAlert` 를 허용하고, 라벨이 없을 때 기본값은 `RealTimeAlert` 입니다.

## Alertmanager 연동 메모

- `upm-monitoring` 네임스페이스의 `kube-prometheus-stack`는 Alertmanager 설정을 `ConfigMap`이 아니라 아래 `Secret`에 저장합니다.
  - `secret/alertmanager-kube-prometheus-stack-alertmanager`
  - key: `alertmanager.yaml`
- 기본 라우팅이 `receiver: "null"` 인 경우 Alerta로 전달되지 않습니다.
- 운영 반영(2026-02-19):
  - `receiver: alerta`
  - `receivers.alerta.webhook_configs.url: http://upm-alarm-alerta.upm-alarm.svc.cluster.local:8080/api/webhooks/prometheus`
  - `route.group_by: [namespace, alertname]`
  - `route.group_wait: 5s`
  - `route.group_interval: 15s`
  - `Watchdog`만 `null` receiver 유지

예시(개념):

```yaml
receivers:
- name: alerta
  webhook_configs:
  - url: "http://upm-alarm-alerta.upm-alarm.svc.cluster.local:8080/api/webhooks/prometheus"
    send_resolved: true

route:
  receiver: alerta
```

## Helm 영구 반영 메모 (kube-prometheus-stack)

런타임에서 Secret만 패치하면 재배포 시 덮일 수 있으므로, 아래처럼 Helm 릴리스에 영구 반영합니다.

```bash
helm upgrade kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --version 81.2.2 \
  -n upm-monitoring \
  --reuse-values \
  -f /tmp/kps-alertmanager-override.yaml
```

검증:

```bash
helm get values -n upm-monitoring kube-prometheus-stack -o yaml
oc get secret -n upm-monitoring alertmanager-kube-prometheus-stack-alertmanager -o jsonpath='{.data.alertmanager\.yaml}' | base64 -d
oc exec -n upm-monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- cat /etc/alertmanager/config_out/alertmanager.env.yaml
```

## Local EMS Alerta profile

The local deployment in this repository uses the following Alerta settings:

- `replicaCount: 2`.
- `deploymentStrategy.type: RollingUpdate` with `maxSurge: 1` and `maxUnavailable: 1`.
- `nodeSelector.node-group.ntels.io/type: upm-ems` to place Alerta on EMS nodes only.
- `tolerations` with `NoExecute` and `tolerationSeconds: 10` for fast node eviction on `not-ready` and `unreachable`.
- `podAntiAffinity.type: required` with `topologyKey: kubernetes.io/hostname`.
- `env.SQLALCHEMY_POOL_RECYCLE: 300` for PgBouncer-friendly connection recycling.
- `secret.stringData.DATABASE_URL: postgresql://upmalert:upmalert@upmdb-pooler.upm-database.svc.cluster.local:5432/upmalert`.

## PostgreSQL / PgBouncer note

Alerta can connect to the existing PostgreSQL cluster through `upmdb-pooler`, but the following must already exist in PostgreSQL:

- database: `upmalert`
- user: `upmalert`
- password: `upmalert`

If the role or database does not exist yet, the Helm chart will render correctly but Alerta will fail to connect at runtime.

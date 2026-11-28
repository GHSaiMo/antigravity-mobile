# Multigravity (`mgy`) セルフホストリレー（BYO-Relay）構築ガイド

> 🌐 [简体中文](self_hosted_relay_guide.md) | [English](self_hosted_relay_guide_en.md) | **日本語**

Multigravity の外出先リモート通信を**すべてご自身のサーバー（VPS / クラウドサーバー）経由**で運用したい場合、内蔵の **BYO-Relay（Bring Your Own Relay）モード**を利用できます。

- **ソースコード改変・スマホアプリの再ビルド不要**：App Store / TestFlight / GitHub Release の公式アプリをそのまま利用できます。
- **Cloudflare Worker ディスパッチャー不要**：リレー本体 `mgy-relay` にオフラインでトークンを発行できるサブコマンドが内蔵されています。
- **公式リレーから完全独立**：設定後、PC 側の `mgy` は公式ディスパッチャーへ接続せず `cloudflared` もダウンロードしません。スマホ側も切断時に公式リレープールへ問い合わせることはありません。
- **ゼロ知識エンドツーエンド暗号化（`mgy-e2ee-v1`）を強制**：自前 VPS 経由でも中継されるのは AES-256-GCM 暗号文のみで、平文リクエストは PC 側ゲートウェイが `HTTP 426` で即座に拒否します。

> **動作要件**：PC 側ゲートウェイ `mgy >= v1.2.8`、スマホ側 iOS / Android アプリ `>= v1.2.8`。

---

## 目次

1. [全体構成と事前準備](#1-全体構成と事前準備)
2. [パート 1：自前リレーサーバー（VPS）の構築](#2-パート-1自前リレーサーバーvpsの構築)
3. [パート 2：PC 側ゲートウェイ（`mgy`）の設定](#3-パート-2pc-側ゲートウェイmgyの設定)
4. [パート 3：スマホアプリ・Web 側の設定](#4-パート-3スマホアプリweb-側の設定)
5. [パート 4：複数 PC の収容・APNs 通知とプライバシー強化](#5-パート-4複数-pc-の収容apns-通知とプライバシー強化)
6. [フラグ・環境変数リファレンス](#6-フラグ環境変数リファレンス)
7. [トラブルシューティング（FAQ）](#7-トラブルシューティングfaq)

---

## 1. 全体構成と事前準備

### 1.1 通信の仕組み

```text
スマホ (iOS / Android / Web)
   │  HTTPS / WSS（アプリ層 AES-256-GCM エンドツーエンド暗号文）
   ▼  トンネルドメインへアクセス：https://dev.example.com
自前リレーサーバー (mgy-relay)
   ▲  制御ドメインへ逆接続：wss://ctl.example.com/_mgy_tunnel（HMAC トークン認証 + yamux 多重化）
   │
ご自身の PC (mgy ゲートウェイ) ──▶ ローカル Antigravity IDE / CLI
```

### 1.2 必要なもの

1. **グローバル IP を持つ Linux サーバー（VPS）**：最小 1 vCPU / 512MB RAM で十分動作します（`mgy-relay` のアイドル時メモリ消費は約 10 MB）。
2. **独自ドメインと 2 つの DNS レコード**（以下は `example.com` の例です。ご自身のドメインに読み替えてください）：

| 役割 | ドメイン例 | DNS レコード | 用途 |
| :--- | :--- | :--- | :--- |
| **制御ドメイン** | `ctl.example.com` | `A` → VPS の IP | PC 側 `mgy` からの逆接続（`/_mgy_tunnel`）およびヘルスチェック（`/healthz`） |
| **トンネルドメイン** | `dev.example.com` | `A` → VPS の IP | スマホ接続・QR ペアリング用（複数 PC を繋ぐ場合はワイルドカード `*.mgy.example.com` が便利） |

> ⚠️ **必ず別々のドメインを 2 つ用意してください**：`mgy-relay` は `MGY_RELAY_CONTROL_HOSTS` に指定した制御ドメインを自動的に予約ホスト扱いとするため、**制御ドメインとトンネルドメインに同じホスト名は使えません**。

3. **ファイアウォール設定**：TCP **`443`** のインバウンドを許可してください。Caddy で自動的に証明書を取得する場合は TCP **`80`** も許可します。

---

## 2. パート 1：自前リレーサーバー（VPS）の構築

### 2.1 `mgy-relay` 単体バイナリをビルドする

現在 GitHub Releases で配布されているバイナリは PC 向け `mgy` のみのため、リレー本体 `mgy-relay` はソースから静的ビルドします（約 7 MB、外部依存なし）。手元の PC でクロスコンパイルして VPS に転送しても、VPS 上で直接ビルドしても構いません：

```bash
git clone https://github.com/GHSaiMo/antigravity-mobile.git
cd antigravity-mobile

# Linux x86_64 サーバー向け：
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o mgy-relay ./cmd/relay

# ARM64 サーバー（Oracle ARM / Raspberry Pi / AWS Graviton 等）向け：
# CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o mgy-relay ./cmd/relay
```

ビルドしたバイナリを VPS の `/opt/mgy-relay/mgy-relay` に配置します：

```bash
sudo mkdir -p /opt/mgy-relay /etc/mgy-relay
sudo mv mgy-relay /opt/mgy-relay/mgy-relay
sudo chmod +x /opt/mgy-relay/mgy-relay
```

---

### 2.2 署名シークレット生成と環境ファイルの作成

VPS 上で 256-bit のランダムな署名キーを生成します（トークン検証に使うため厳重に保管してください）：

```bash
openssl rand -hex 32
```

設定ファイル `/etc/mgy-relay/relay.env` を作成します（root のみ読み取り可能）：

```bash
sudo tee /etc/mgy-relay/relay.env > /dev/null <<'EOF'
# 上で生成した 64 文字の 16 進シークレットに置き換えてください
MGY_RELAY_SIGNING_SECRET=生成した64文字のシークレット

# 制御ドメイン（トンネルドメインとは別にする）
MGY_RELAY_CONTROL_HOSTS=ctl.example.com

# 許可するトンネルドメインのサフィックス（デフォルト .jiuge.space を必ず上書き；空にすると非予約ドメイン全般を許可）
MGY_RELAY_ALLOWED_SUFFIXES=.example.com

# リッスン設定（2.3 節で選ぶ HTTPS 方式に合わせて調整）
MGY_RELAY_LISTEN=127.0.0.1:8443
MGY_RELAY_TLS_CERT=
MGY_RELAY_TLS_KEY=
EOF

sudo chmod 600 /etc/mgy-relay/relay.env
```

> ⚠️ **よくある落とし穴**：デフォルト値は `MGY_RELAY_ALLOWED_SUFFIXES=.jiuge.space`、`MGY_RELAY_CONTROL_HOSTS=relay.jiuge.space` です。**両方ともご自身のドメインに上書きしないと**、PC 接続時に `403 domain_not_allowed` エラーになります。

---

### 2.3 HTTPS の設定（どちらか一方を選択）

#### 方式 A（おすすめ・最も簡単）：Caddy / Nginx リバースプロキシの背後に配置

上記の `/etc/mgy-relay/relay.env` のまま（ローカル HTTP 待ち受け）にします：

```env
MGY_RELAY_LISTEN=127.0.0.1:8443
MGY_RELAY_TLS_CERT=
MGY_RELAY_TLS_KEY=
```

- **Caddy を使う場合（自動 HTTPS 取得・更新対応で最も手軽）**：

  `/etc/caddy/Caddyfile` に以下を追加：
  ```caddyfile
  ctl.example.com, dev.example.com {
      reverse_proxy 127.0.0.1:8443
  }
  ```
  反映：`sudo systemctl reload caddy`

- **Nginx を使う場合**：

  > `mgy-relay` は `Host` ヘッダーで各 PC のトンネルを振り分けるため、元の `Host` 転送・WebSocket Upgrade・バッファ無効化・長めのタイムアウトが必須です。

  ```nginx
  map $http_upgrade $connection_upgrade {
      default upgrade;
      ''      close;
  }

  server {
      listen 443 ssl http2;
      server_name ctl.example.com dev.example.com;

      ssl_certificate     /path/to/fullchain.pem;
      ssl_certificate_key /path/to/privkey.pem;
      ssl_protocols       TLSv1.2 TLSv1.3;

      client_max_body_size 64m;

      location / {
          proxy_pass http://127.0.0.1:8443;
          proxy_http_version 1.1;

          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;

          # 逆接続 WSS トンネルおよびストリーミング出力に必須
          proxy_set_header Upgrade $http_upgrade;
          proxy_set_header Connection $connection_upgrade;
          proxy_buffering off;
          proxy_read_timeout 3600s;
          proxy_send_timeout 3600s;
      }
  }
  ```
  構文チェック＆反映：`sudo nginx -t && sudo systemctl reload nginx`

---

#### 方式 B（最小構成・リバースプロキシ不要）：`mgy-relay` で直接 443 を待ち受け

VPS の 443 番ポートが空いており、`acme.sh` や `certbot` で `ctl.example.com` と `dev.example.com` の両方（またはワイルドカード `*.example.com`）を含む証明書を用意済みの場合は、`/etc/mgy-relay/relay.env` を以下のように変更します：

```env
MGY_RELAY_LISTEN=:443
MGY_RELAY_TLS_CERT=/etc/mgy-relay/fullchain.pem
MGY_RELAY_TLS_KEY=/etc/mgy-relay/privkey.pem
```

> ヒント：`acme.sh` の自動更新時に `--reloadcmd "systemctl restart mgy-relay"` を指定すると、更新後の証明書が自動反映されます。

---

### 2.4 `systemd` サービス登録と起動

`/etc/systemd/system/mgy-relay.service` を作成します：

```bash
sudo tee /etc/systemd/system/mgy-relay.service > /dev/null <<'EOF'
[Unit]
Description=Multigravity Zero-Knowledge E2EE Reverse Relay
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/mgy-relay/relay.env
ExecStart=/opt/mgy-relay/mgy-relay
Restart=always
RestartSec=2
LimitNOFILE=65536
Environment=GOMEMLIMIT=450MiB

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now mgy-relay
sudo systemctl status mgy-relay --no-pager
```

### 2.5 リレーサーバーの疎通確認

任意の端末から制御ドメインの `/healthz` を叩きます：

```bash
curl -s https://ctl.example.com/healthz
```

以下のような JSON が返ればサーバーと HTTPS の準備は完了です：

```json
{"active_conns":0,"active_streams":0,"active_tunnels":0,"service":"mgy-relay","status":"ok","total_streams":0,"uptime_sec":12,"version":"1.1.0"}
```

---

### 2.6 オフライン接続トークンの発行（Worker 不要）

VPS 上で内蔵の `token` コマンドを実行し、PC 用の署名済みトークンを発行します：

```bash
sudo bash -c '
  set -a
  source /etc/mgy-relay/relay.env
  set +a
  /opt/mgy-relay/mgy-relay token \
    --domain dev.example.com \
    --endpoint wss://ctl.example.com/_mgy_tunnel
'
```

`mgyr1.*` トークンと、PC 側にそのまま貼れる 3 行の環境変数が出力されます：

```env
# Add to your gateway ~/.multigravity/.env (BYO-Relay self-hosted mode):
MGY_CUSTOM_RELAY_ENDPOINT=wss://ctl.example.com/_mgy_tunnel
MGY_CUSTOM_RELAY_TOKEN=mgyr1.eyJ2IjoxLCJzaWQiOi....xxxxxxxx
MGY_CUSTOM_RELAY_PUBLIC_URL=https://dev.example.com
```

この 3 行を控えて次のパートへ進みます。

---

## 3. パート 2：PC 側ゲートウェイ（`mgy`）の設定

### 3.1 `mgy` のバージョンを確認（`>= v1.2.8`）

Antigravity を動かしている PC のターミナルで確認します：

```bash
mgy version
```

`v1.2.8` 未満の場合はアップデートしてください：

```bash
mgy update
```

### 3.2 `.env` に自前リレー設定を追記

PC 側の設定ファイルを開き（なければ新規作成）：
- **macOS / Linux**：`~/.multigravity/.env`
- **Windows**：`%USERPROFILE%\.multigravity\.env`

パート 1 の 2.6 節で生成した 3 行を追記して保存します：

```env
# セルフホスト・ゼロ知識 E2EE リレー（BYO-Relay）
MGY_CUSTOM_RELAY_ENDPOINT=wss://ctl.example.com/_mgy_tunnel
MGY_CUSTOM_RELAY_TOKEN=mgyr1.eyJ2IjoxLCJzaWQiOi....xxxxxxxx
MGY_CUSTOM_RELAY_PUBLIC_URL=https://dev.example.com
```

> 補足：
> - `MGY_CUSTOM_RELAY_ENDPOINT` と `MGY_CUSTOM_RELAY_TOKEN` の両方が設定されていると、`mgy` は**自動的に BYO-Relay モードへ切り替わり**、公式ディスパッチャー（`dispatcher.multigravity.app`）への接続や `cloudflared` のダウンロードを行いません（`CF_TUNNEL_ENABLED=0` を別途設定する必要はありません）。
> - `MGY_CUSTOM_RELAY_PUBLIC_URL` は省略も可能で、省略時はトークン内の `dom` から自動的に `https://dev.example.com` が補完されます。

### 3.3 ゲートウェイを再起動してログを確認

- **フォアグラウンド起動の場合**：元のターミナルで `Ctrl + C` を押して終了し、再起動：
  ```bash
  mgy
  ```
- **バックグラウンドサービス（`mgy service`）の場合**：
  ```bash
  mgy service restart
  mgy service logs -f
  ```

起動バナーまたはログに以下のように表示されれば、自前リレーへの接続は成功です：

```text
  ➜  Cloud Tunnel: Self-hosted E2EE relay active (https://dev.example.com | BYO-Relay)
```

### 3.4 エンドツーエンドの疎通確認（推奨）

任意の端末から次の 2 つを実行して確認します：

```bash
# 1. active_tunnels が 1、active_conns が 1 以上になっていることを確認
curl -s https://ctl.example.com/healthz

# 2. トンネルドメインへ平文アクセス → HTTP 426 e2ee_required が返るのが正常
curl -i https://dev.example.com/api/v1/version
```

2 つ目で **`HTTP/2 426`** と `{"error":"e2ee_required", ...}` が返れば、「外部回線 → 自前 VPS → PC 側ゲートウェイ」の貫通と強制 E2EE 保護の両方が正しく機能しています。

---

## 4. パート 3：スマホアプリ・Web 側の設定

スマホ側は**アプリの改造や再ビルド不要**です。公式の **iOS（App Store / TestFlight）** または **Android（GitHub Release APK）** アプリ（`>= v1.2.8`）をそのまま利用します。

### 4.1 パターン A：初めてペアリングする場合

1. PC 側で `mgy pair` を実行（または起動直後の QR コードを表示）します。QR コードには自前ドメイン `https://dev.example.com`、LAN アドレス、E2EE 公開鍵が自動的に埋め込まれています。
2. スマホアプリで QR コードをスキャンします。
3. 1〜2 秒でペアリングが完了し、手入力なしでそのまま利用できます。

### 4.2 パターン B：すでに公式リレーでペアリング済みのスマホを移行する場合

次のいずれかの方法で自前ドメインへ紐付け直してください：

- **方法 1（おすすめ・他の登録ホストを保持）**：
  1. PC 側で `mgy pair` を実行して新しい QR コードを表示；
  2. スマホアプリのホーム上部にあるホスト名をタップして **ホストを切替** パネルを開き、**「+ ホスト追加」** をタップ；
  3. 新しい QR コードをスキャンすると、同一ゲートウェイ ID として自前リレー URL と E2EE 対象ホストが上書き更新されます。
- **方法 2（完全にやり直す場合）**：
  1. スマホの **設定 → デバイスペアリング → ペアリングを解除** を実行；
  2. PC 側で `mgy pair` を実行；
  3. 新しく QR コードをスキャンして再ペアリングします。

### 4.3 自前サーバー経由で通信しているか確認する

1. **スマホの Wi-Fi を切り、5G / 4G モバイル通信に切り替え**ます（LAN 直結を除外するため）。
2. アプリで会話を開くかメッセージを送信し、ストリーミング表示されることを確認します。
3. VPS 側でヘルスチェックを確認します：
   ```bash
   curl -s https://ctl.example.com/healthz
   ```
   操作に応じて `total_streams` が増えていれば、すべてのリモート通信が自前 VPS 経由で流れています。

### 4.4 スマホ側の自動ルーティング優先順位

```text
① 同一 Wi-Fi の LAN 直結（自宅・社内では超低遅延、VPS 帯域を消費しない）
   ↓ 外出 / 5G へ切替
② 自前 E2EE リレー（https://dev.example.com、常時 AES-256-GCM 暗号化）
```

> スマホアプリは独自ドメインを自動判別して隔離します。自前ドメイン運用中に VPS が一時停止しても、**公式ディスパッチャー（`dispatcher.multigravity.app`）へ問い合わせることは一切ありません**。

### 4.5 Web / PWA からの利用

リレー上の Web を無効にしていない場合、任意のブラウザで `https://dev.example.com` を開き、`mgy pair` の 6 桁ペアリングコードを入力すればそのまま利用できます（Web Crypto API による E2EE 暗号化対応）。

---

## 5. パート 4：複数 PC の収容・APNs 通知とプライバシー強化

### 5.1 1 台の VPS で複数台の PC をまとめて中継する

`mgy-relay` は 1 プロセスで複数 PC を同時収容できます（Worker 不要）：
1. DNS にワイルドカードレコード `*.mgy.example.com` → VPS の IP を登録し、ワイルドカード証明書を設定；
2. `/etc/mgy-relay/relay.env` に `MGY_RELAY_ALLOWED_SUFFIXES=.mgy.example.com` を設定；
3. PC ごとに異なる `--domain` でトークンを発行：
   ```bash
   # 会社 Mac 用
   /opt/mgy-relay/mgy-relay token --domain mac.mgy.example.com --endpoint wss://ctl.example.com/_mgy_tunnel
   # 自宅 Windows 用
   /opt/mgy-relay/mgy-relay token --domain win.mgy.example.com --endpoint wss://ctl.example.com/_mgy_tunnel
   ```
4. 各 PC の `~/.multigravity/.env` にそれぞれ貼り付けて再起動し、スマホの **「+ ホスト追加」** からスキャンすれば 1 台のスマホで自在に切り替えられます。

### 5.2 iOS プッシュ通知（APNs）と「公式通信完全ゼロ」設定

BYO-Relay を有効にすると、**会話・コード差分・ファイル・端末出力は 100% 自前サーバー経由のみ**になります。唯一オプションで公式エンドポイントを使い得るのは、iOS App Store 版のリモート通知・ライブアクティビティ更新（`https://push.multigravity.app`）です（Apple の仕様上、APNs `.p8` 鍵が公式 Bundle ID に紐づくため）。

プライバシー要件に応じて PC 側 `~/.multigravity/.env` で以下から選べます：

| ポリシー | 設定項目 | 動作 |
| :--- | :--- | :--- |
| **通知を維持（デフォルト）** | 設定不要 | ロック画面承認・ライブアクティビティ対応（プッシュ中継は完全ステートレス・DB/ログなし） |
| **通知を維持＋本文秘匿（推奨）** | `MGY_PUSH_REDACT=1` | 会話タイトル・コマンド・ファイルパス・ホスト名を削除し、通知種別と進捗数値（例 `3/5`）のみ送信 |
| **公式エンドポイントへの通信を完全遮断** | `APNS_ENABLED=0` | 公式ドメインへの通信を **0 バイト** に完全停止（APNs も自前化する場合は個人の Apple Developer アカウントで iOS アプリを再署名し、[scripts/cloudflare-apns-worker/](../scripts/cloudflare-apns-worker/README.md) をデプロイ） |

### 5.3 追加のセキュリティ強化オプション（PC 側 `.env`）

```env
# ネイティブアプリ専用モード：リレー上の Web/PWA 画面を閉じ、/healthz の公開鍵・ホスト情報を非表示化
MGY_RELAY_DISABLE_WEB=1

# 厳格なリプレイ攻撃防御：全スマホを v1.2.8+ に更新後、タイムスタンプ／req_id なしの旧 E2EE リクエストを拒否
MGY_E2EE_STRICT_REPLAY=1
```

### 5.4 発展：複数 VPS による自動速度計測・フェイルオーバープール

複数リージョンに自前 VPS を展開して自動フェイルオーバーさせたい場合は、[scripts/cloudflare-worker/worker.js](../scripts/cloudflare-worker/worker.js) をご自身の Cloudflare 無料アカウントにデプロイし（`RELAY_POLICY=prefer` と `RELAY_POOL` を設定）、PC 側 `.env` の `MGY_CUSTOM_RELAY_*` を外して以下を指定してください：

```env
CF_WORKER_URL=https://your-own-dispatcher.example.com
```

---

## 6. フラグ・環境変数リファレンス

### 6.1 リレーサーバー `mgy-relay`（VPS）

| 環境変数 | CLI フラグ | デフォルト値 | 説明 |
| :--- | :--- | :--- | :--- |
| `MGY_RELAY_SIGNING_SECRET` | — | *(必須)* | `mgy-relay token` と共有する HMAC-SHA256 署名キー |
| `MGY_RELAY_LISTEN` | `-listen` | `:443` | 待ち受けアドレス（Caddy/Nginx 配下では `127.0.0.1:8443`） |
| `MGY_RELAY_TLS_CERT` | `-tls-cert` | `/etc/mgy-relay/origin.crt` | TLS フルチェーン証明書パス（**リバースプロキシ配下では空文字**） |
| `MGY_RELAY_TLS_KEY` | `-tls-key` | `/etc/mgy-relay/origin.key` | TLS 秘密鍵パス |
| `MGY_RELAY_CONTROL_HOSTS` | `-control-hosts` | `relay.jiuge.space` | 制御ドメイン（カンマ区切り、**自前運用時は変更必須**） |
| `MGY_RELAY_ALLOWED_SUFFIXES` | `-allowed-suffixes` | `.jiuge.space` | 許可トンネルドメインサフィックス（**自前運用時は変更必須**、空文字で非予約ドメイン全般を許可） |
| `MGY_RELAY_RESERVED_HOSTS` | `-reserved-hosts` | `""` | トンネル利用を禁止する追加ホスト名（カンマ区切り） |
| `MGY_RELAY_MAX_TUNNELS` | `-max-tunnels` | `10000` | 最大同時接続ゲートウェイ数 |
| `MGY_RELAY_MAX_STREAMS_PER_TUNNEL` | `-max-streams` | `64` | 1 トンネルあたりの最大同時 HTTP/WS ストリーム数 |

### 6.2 PC 側ゲートウェイ `mgy`（`~/.multigravity/.env`）

| 環境変数 | 必須 | 設定例 | 説明 |
| :--- | :---: | :--- | :--- |
| `MGY_CUSTOM_RELAY_ENDPOINT` | はい | `wss://ctl.example.com/_mgy_tunnel` | 制御用 WebSocket エンドポイント（`https://ctl.example.com` と書いても自動補完されます） |
| `MGY_CUSTOM_RELAY_TOKEN` | はい | `mgyr1.xxxx.yyyy` | `mgy-relay token` で発行したオフライン署名トークン |
| `MGY_CUSTOM_RELAY_PUBLIC_URL` | いいえ | `https://dev.example.com` | スマホ・Web 向け公開 URL（省略時はトークン内の `dom` から自動導出） |
| `MGY_PUSH_REDACT` | いいえ | `1` | プッシュ通知本文の秘匿化 |
| `APNS_ENABLED` | いいえ | `0` | 公式 APNs 通知中継の完全無効化 |
| `MGY_RELAY_DISABLE_WEB` | いいえ | `1` | リレー経由の Web/PWA 画面を無効化（ネイティブアプリ専用） |

---

## 7. トラブルシューティング（FAQ）

1. **PC 側ログに `403 domain_not_allowed` が出る**
   - VPS の `/etc/mgy-relay/relay.env` で `MGY_RELAY_ALLOWED_SUFFIXES` がデフォルトの `.jiuge.space` のままになっていないか、トンネルドメインと一致しているか確認してください。
   - **制御ドメイン**と**トンネルドメイン**に同じホスト名を使っていないか確認してください（制御ドメインは自動的に予約済みとして拒否されます）。

2. **PC 側ログに `401 invalid_token` が出る**
   - `mgy-relay token` 実行時の `MGY_RELAY_SIGNING_SECRET` と稼働中 `mgy-relay` のシークレットが一致していないか、`.env` コピー時に余分な改行や空白が混入しています。同じキーで再発行し再起動してください。

3. **`https://dev.example.com` へアクセスすると `503 {"error":"gateway_offline"}` が返る**
   - DNS・HTTPS・リバースプロキシ自体は正常ですが、PC 側の `mgy` が VPS に接続できていない（または PC がスリープ／停止中）状態です。
   - PC 側ログ（`mgy service logs -f`）および VPS 側ログ（`sudo journalctl -u mgy-relay -n 50 --no-pager`）と、`https://ctl.example.com/healthz` の `active_tunnels` が `1` か確認してください。

4. **Nginx 経由だと 60 秒ごとに切断される、または AI の応答が途中で止まる**
   - Nginx の `location /` に `proxy_http_version 1.1;`、`Upgrade` / `Connection` ヘッダー、`proxy_read_timeout 3600s;`、`proxy_buffering off;` が入っているか確認してください。
   - `proxy_set_header Host $host;` が抜けていると `mgy-relay` に正しいドメインが届かず `unknown_host` になります。

5. **`curl https://dev.example.com/api/v1/version` で `HTTP 426 e2ee_required` が返るのはエラーですか？**
   - **いいえ、正常なセキュリティ動作です。** 自前リレーから PC 側ゲートウェイまで開通しており、平文アクセスを正しくブロックできている証拠です。スマホアプリで QR ペアリングすれば自動的に E2EE 暗号化通信で接続されます。

6. **後から公式の無料リレーに戻したい場合は？**
   - PC 側の `~/.multigravity/.env` から `MGY_CUSTOM_RELAY_*` の 3 行を削除またはコメントアウトして `mgy` を再起動し、スマホで `mgy pair` の QR コードを一度スキャンし直すだけで元に戻せます。

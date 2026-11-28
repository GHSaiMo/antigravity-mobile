# Multigravity (`mgy`) IPv6 直結設定ガイド

> 🌐 [简体中文](ipv6_direct_guide.md) | [English](ipv6_direct_guide_en.md) | **日本語**

自宅の光回線・学内ネットワーク・社内網・クラウドサーバーで **IPv6** が利用できる場合、Multigravity ゲートウェイの **IPv4/IPv6 デュアルスタック待受**（および任意のリバースプロキシ不要の**ネイティブ HTTPS ポート**）を有効にできます。5G/4G や IPv6 対応ネットワークからは低遅延・高帯域で PC へ直接接続でき、IPv6 非対応の社内 Wi-Fi などでは自動的に公式のゼロ知識 E2EE リレーへフォールバックして接続を維持します。

> **動作要件**：PC 側ゲートウェイ `mgy >= v1.2.9-hotfix.1`（および今後の `v1.2.9+` 正式版）。**デフォルトは無効（IPv4 専用 `tcp4`）** のオプトイン方式のため、既存環境への影響はありません。  
> **スマホ側**：**DDNS + HTTPS** 方式の直結であれば、iOS / Android アプリのアップデート不要でそのまま利用できます。

---

## 目次

1. [ゲートウェイをアップデートする](#1-ゲートウェイをアップデートする)
2. [3 つの直結方式の選び方](#2-3-つの直結方式の選び方)
3. [事前確認：PC の IPv6 とルーターのファイアウォール](#3-事前確認pc-の-ipv6-とルーターのファイアウォール)
4. [方式 1（推奨）：DDNS ＋ ゲートウェイ標準 HTTPS（リバースプロキシ不要）](#4-方式-1推奨ddns--ゲートウェイ標準-httpsリバースプロキシ不要)
5. [方式 2：DDNS ＋ リバースプロキシ（Caddy / Nginx / Lucky）](#5-方式-2ddns--リバースプロキシcaddy--nginx--lucky)
6. [方式 3：Tailscale / プライベート LAN IPv6 直結](#6-方式-3tailscale--プライベート-lan-ipv6-直結)
7. [スマホアプリ側の設定と自動ルーティング優先順位](#7-スマホアプリ側の設定と自動ルーティング優先順位)
8. [応用設定：公式リレー無効化 / E2EE 強制 / 自動起動サービス](#8-応用設定公式リレー無効化--e2ee-強制--自動起動サービス)
9. [コマンド引数・環境変数リファレンス](#9-コマンド引数環境変数リファレンス)
10. [よくある質問とトラブルシューティング（FAQ）](#10-よくある質問とトラブルシューティングfaq)

---

## 1. ゲートウェイをアップデートする

`mgy` を実行している PC のターミナルで以下を実行します：

```bash
# IPv6 デュアルスタック・ネイティブ TLS 対応版へ更新
mgy update v1.2.9-hotfix.1

# すでに初期の v1.2.9-hotfix.1 を導入済みの場合は -force を付けて上書き更新
mgy update -force v1.2.9-hotfix.1
```

新しいオプションが有効になっていることを確認します：

```bash
mgy help | grep -E "ipv6|tls-"
```

`-ipv6`、`-tls-cert`、`-tls-key`、`-tls-port` が表示されれば準備完了です。

---

## 2. 3 つの直結方式の選び方

| 方式 | 向いている環境 | ドメイン・証明書が必要か | おすすめ度 |
| :--- | :--- | :--- | :--- |
| **方式 1：DDNS ＋ ゲートウェイ標準 HTTPS** | ドメインと ACME 証明書があり、Nginx/Caddy を別途立てたくない | 必要（`mgy` 単体で配信・更新時の自動ホットリロード対応） | ⭐⭐⭐⭐⭐ 最もシンプル |
| **方式 2：DDNS ＋ Caddy / Nginx / Lucky** | PC・NAS・ルーター上で既にリバースプロキシを運用している | リバースプロキシ側で自動取得・更新 | ⭐⭐⭐⭐⭐ 既存プロキシ環境向け |
| **方式 3：Tailscale / プライベート IPv6** | Tailscale を利用中、または LAN 内 IPv6 のみ使いたい | 不要（暗号化トンネルまたは LAN 内通信） | ⭐⭐⭐⭐ ドメイン不要 |

> **なぜドメイン・証明書なしのグローバル IPv6 平文 HTTP（例：`http://[2408:...]:58900`）は非対応なのですか？**
> 1. **セキュリティ**：インターネット上で平文 HTTP を流すと、デバイス認証トークン（Bearer Token）や会話内容が露出します。
> 2. **OS・アプリの制約**：iOS の App Transport Security（ATS）および Android の平文通信遮断インターセプターにより、パブリックアドレスへの平文 HTTP はブロックされます。
> 3. **アドレス変動対策**：家庭用回線の IPv6 プレフィックスや OS の一時プライバシーアドレスは定期的に変わるため、DDNS ドメイン（AAAA レコード）を使うことで再ペアリング不要で安定運用できます。

---

## 3. 事前確認：PC の IPv6 とルーターのファイアウォール

### 3.1 PC がグローバル IPv6 アドレスを取得しているか確認

- **macOS / Linux**：
  ```bash
  curl -6 -s https://api64.ipify.org && echo
  ```
- **Windows (PowerShell)**：
  ```powershell
  curl.exe -6 -s https://api64.ipify.org
  ```

`2001:...`、`2400:...`、`2408:...`、`2409:...`、`240e:...` などのグローバルアドレスが返れば、PC の IPv6 は有効です。

### 3.2 ルーターの IPv6 ファイアウォール許可（最も多いつまずきポイント）

家庭用ルーターや HGW / ONT はデフォルトで **IPv6 ファイアウォール（IPv6 パケットフィルタ）** が有効になっており、外部からの着信 TCP 接続を遮断します。

- **安全な設定**：ルーターの IPv6 ファイアウォール設定で、**HTTPS 用ポートの TCP 着信のみを許可**してください（例：方式 1 の `58943`、または方式 2 のリバースプロキシポート）。**HTTP ポート `58900` は外部公開せず、IPv6 ファイアウォール自体も無効化しないでください。**
- **80 / 443 番ポートの回避**：家庭用 ISP によっては着信 `80` / `443` が遮断される場合があるため、デフォルトの **`58943`** などのハイポート利用を推奨します。

---

## 4. 方式 1（推奨）：DDNS ＋ ゲートウェイ標準 HTTPS（リバースプロキシ不要）

リバースプロキシを用意せず、`mgy` 自身で専用ポート（デフォルト `58943`）に **TLS 1.2+** を直接待ち受けます。従来の HTTP ポート（`58900`）はそのまま維持されるため、ローカル CLI（`mgy pair` / `list` / `clear`）や自宅 Wi-Fi LAN 接続に影響しません。

### ステップ 1：DDNS（AAAA レコード）と PEM 証明書を用意する

- `ddns-go`、Lucky、ルーター標準の DDNS、または DNS プロバイダー API で、独自ドメイン（例：`home.example.com`）の **AAAA レコード** を PC のグローバル IPv6 に紐付けます。
- `acme.sh`、`certbot`、`ddns-go`、Lucky などで Let's Encrypt / ZeroSSL 証明書を発行し、以下の 2 ファイルを用意します：
  - フルチェーン証明書：`fullchain.pem`（**中間証明書を含むフルチェーンを指定してください。`cert.pem` 単体は不可**）
  - 秘密鍵：`privkey.pem`

### ステップ 2：`~/.multigravity/.env` を編集する

`~/.multigravity/.env`（Windows は `%USERPROFILE%\.multigravity\.env`）に以下を追記します：

```env
# IPv4 / IPv6 デュアルスタック待受を有効化
MULTIGRAVITY_ENABLE_IPV6=1

# 専用ポートでネイティブ HTTPS を有効化（ACME 証明書更新時の自動ホットリロード対応）
MGY_TLS_CERT=/path/to/fullchain.pem
MGY_TLS_KEY=/path/to/privkey.pem
MGY_TLS_PORT=58943

# 任意：起動バナーに DDNS ドメインを表示したい場合
DDNS_HOST=home.example.com
```

コマンド引数で一時的に試すこともできます：

```bash
mgy -ipv6 -tls-cert /path/to/fullchain.pem -tls-key /path/to/privkey.pem -tls-port 58943 -ddns home.example.com
```

### ステップ 3：ゲートウェイを再起動して起動バナーを確認

`mgy` を再起動すると、バナーに以下の 2 行が表示されます：

```text
  ➜  Listen Stack: IPv4 / IPv6 dual-stack enabled (:58900 | Tailscale/proxy/DDNS direct ready)
  ➜  Native HTTPS: https://home.example.com:58943 (TLS 1.2+, auto-reload cert, listening on :58943)
```

### ステップ 4：外部からの IPv6 HTTPS 疎通テスト

ルーターで TCP `58943` の着信を許可した後、別の IPv6 端末やモバイル回線から以下を確認します：

```bash
curl -6 -i https://home.example.com:58943/healthz
```

`HTTP/2 200`（または `HTTP/1.1 200`）と `{"status":"ok", ...}` が返れば疎通完了です。

> **証明書の自動更新について**：`mgy` は 30 秒ごとに証明書・秘密鍵の更新時刻とサイズをチェックし、ACME ツールが証明書を更新すると**再起動なしで自動的にホットリロード**します。書き込み途中でファイルが一時的に不完全だった場合も、以前の有効な証明書を保持して通信を維持します。

---

## 5. 方式 2：DDNS ＋ リバースプロキシ（Caddy / Nginx / Lucky）

すでに Caddy、Nginx、Lucky などで HTTPS 終端と自動証明書更新を行っている場合は、バックエンドを `mgy` の `58900` ポートへ向けるだけで利用できます。

### ステップ 1：デュアルスタック待受が必要か確認

- **リバースプロキシと `mgy` が同じ PC 上にある場合**：バックエンドを `http://127.0.0.1:58900` に向けるだけで動作し、`MULTIGRAVITY_ENABLE_IPV6=1` すら不要です。
- **リバースプロキシが別端末（ルーター / NAS）にあり、IPv6 経由で PC の `mgy` へ転送する場合**：PC 側の `~/.multigravity/.env` に以下を追記して再起動します：
  ```env
  MULTIGRAVITY_ENABLE_IPV6=1
  ```
  バックエンドは `http://[PCのIPv6]:58900`（または同一 LAN 内の `http://PCのLAN_IPv4:58900`）を指定します。

### ステップ 2：代表的なリバースプロキシ設定例

> Multigravity はリアルタイム応答とイベント配信に **WebSocket** を使用するため、`Upgrade` / `Connection` ヘッダーの転送と長めのタイムアウト設定が必要です。

#### Caddy（自動 HTTPS ＋ WebSocket 標準対応で最も手軽）

```caddyfile
home.example.com:58943 {
    reverse_proxy 127.0.0.1:58900
}
```

#### Nginx

```nginx
server {
    listen [::]:58943 ssl http2;
    listen 58943 ssl http2;
    server_name home.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;

    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:58900;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket およびストリーミング応答に必須
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }
}
```

#### Lucky（ソフトルーター / NAS 向け）

1. **Dynamic DNS** で IPv6 の `AAAA` レコードを設定します。
2. **SSL/TLS Certificates** で対象ドメインの ACME 証明書を取得します。
3. **Web Service** ルールを作成します：
   - 待受タイプ：`tcp6`（または `tcp4` + `tcp6`）、待受ポート：`58943` などのハイポート、**TLS** を有効化；
   - サブルールのフロントドメインに `home.example.com`、バックエンドに `http://127.0.0.1:58900`（または LAN 内アドレス）を指定；
   - Lucky は標準で WebSocket を透過するため、保存するだけで利用可能です。

---

## 6. 方式 3：Tailscale / プライベート LAN IPv6 直結

Tailscale の IPv6（`fd7a:115c:a1e0::/48`）または LAN 内プライベート IPv6（ULA `fc00::/7`）で直接接続したい場合：

1. PC 側の `~/.multigravity/.env` でデュアルスタックを有効化（または Tailscale IPv6 のみにバインド）します：
   ```env
   MULTIGRAVITY_ENABLE_IPV6=1
   # Tailscale IPv6 インターフェースのみにバインドする場合：
   # MULTIGRAVITY_HOST=fd7a:115c:a1e0::xxxx
   ```
2. `mgy` を再起動します。
3. スマホアプリの「カスタムサーバーアドレス」に `http://[fd7a:115c:a1e0::xxxx]:58900` を入力します（従来の Tailscale IPv4 `http://100.x.x.x:58900` や MagicDNS `http://ホスト名.tailxxxx.ts.net:58900` も引き続き利用できます）。

---

## 7. スマホアプリ側の設定と自動ルーティング優先順位

1. 未ペアリングの場合は、通常どおり `mgy`（または `mgy pair`）の QR コードをスキャンしてペアリングを完了します。
2. スマホアプリで **設定 → ネットワーク接続 → カスタムサーバーアドレス** を開きます。
3. 直結用 HTTPS アドレスを入力して保存します：
   ```text
   https://home.example.com:58943
   ```
4. 追加のスイッチ操作は不要で、アプリが自動的に以下の優先順位で最適ルートを選択します：

```text
① Wi-Fi LAN 直結（同一 Wi-Fi 内なら最小遅延で優先接続）
   ↓ 外出・5G/4G モバイル回線へ切替
② カスタムサーバーアドレス（ご自身の IPv6 HTTPS 直結ルート）
   ↓ IPv6 非対応の社内 Wi-Fi や制限環境
③ 公式ゼロ知識 E2EE リレー（自動でシームレスにフォールバック）
```

---

## 8. 応用設定：公式リレー無効化 / E2EE 強制 / 自動起動サービス

### 8.1 公式リレーを無効化して完全直結のみにする

公式リレーへのフォールバックを使わず、LAN と自前の IPv6 直結のみで運用したい場合は `~/.multigravity/.env` に追記します：

```env
CF_TUNNEL_ENABLED=0
```

> ヒント：外出先すべてのネットワークで IPv6 が使える確証がない場合は、デフォルトの `1` のままにしておくのがおすすめです。有効にしていても IPv6 直結の優先順位は下がらず、IPv4 のみの Wi-Fi で自動的にフォールバックしてくれます。

### 8.2 リバースプロキシ / CDN 経由でゼロ知識 E2EE を強制する

外部リバースプロキシや CDN を経由する際に、中継先にも会話内容を一切見せたくない場合は以下を追加します：

```env
MGY_FORCE_E2EE=1
```

外部・プロキシ経由の通信はすべて E2EE が必須になります（iOS / Android アプリは `HTTP 426` を検知して自動切替し、ローカル CLI や LAN 直結には影響しません）。

### 8.3 `mgy service` 自動起動サービスと併用する

- `~/.multigravity/.env` に設定を書いた場合は、サービスを再起動するだけで反映されます：
  ```bash
  mgy service restart
  ```
- サービス登録時に起動引数へ `-ipv6` を固定で含めることもできます：
  ```bash
  mgy service install -ipv6
  ```

---

## 9. コマンド引数・環境変数リファレンス

| コマンド引数 | 環境変数（左が優先） | デフォルト | 説明 |
| :--- | :--- | :--- | :--- |
| `-ipv6` | `MULTIGRAVITY_ENABLE_IPV6` / `MGY_ENABLE_IPV6` | `false` (`0`) | IPv4/IPv6 デュアルスタック待受を有効化（既定は IPv4 専用 `tcp4`） |
| `-host <アドレス>` | `MULTIGRAVITY_HOST` | `""` | バインド先アドレス。`::`、`::1`、`[fd7a:...]` など IPv6 を直接指定すると自動的に IPv6 待受が有効化 |
| `-port <番号>` | `MULTIGRAVITY_PORT` | `58900` | メイン HTTP / WebSocket 待受ポート（ローカル CLI・LAN・リバースプロキシ用） |
| `-tls-cert <パス>` | `MGY_TLS_CERT` / `MULTIGRAVITY_TLS_CERT` | `""` | ネイティブ HTTPS フルチェーン証明書パス（PEM）。`-tls-key` とセットで指定 |
| `-tls-key <パス>` | `MGY_TLS_KEY` / `MULTIGRAVITY_TLS_KEY` | `""` | ネイティブ HTTPS 秘密鍵パス（PEM）。ACME 更新時に自動ホットリロード |
| `-tls-port <番号>` | `MGY_TLS_PORT` / `MULTIGRAVITY_TLS_PORT` | `58943` | ネイティブ HTTPS 専用待受ポート（`-port` とは別番号を指定） |
| `-ddns <ドメイン>` | `DDNS_HOST` | `""` | 起動バナー表示やペアリング候補に使う DDNS ドメイン |
| — | `CF_TUNNEL_ENABLED` | `1` | `0` にすると公式クラウドリレーを無効化 |
| — | `MGY_FORCE_E2EE` | `0` | `1` にすると外部・プロキシ経由通信すべてにゼロ知識 E2EE を強制 |

---

## 10. よくある質問とトラブルシューティング（FAQ）

1. **ローカルからは繋がるのに、スマホの 5G 回線からタイムアウトする**
   - ほとんどの原因は **HGW / ONT またはルーターの IPv6 ファイアウォール**による着信遮断です。以下を確認してください：
     - ルーターの上流に ISP 提供の HGW（ルーター機能付き）がある場合、上流側でも対象ポートの着信許可が必要です；
     - ルーターの IPv6 ファイアウォールで対象 TCP ポート（例：`58943`）が許可されているか；
     - DDNS の `AAAA` レコードが PC の**現在有効なグローバル IPv6 アドレス**を指しているか（期限切れの一時プライバシーアドレスやルーター自身の IPv6 を指していないか）。
2. **スマホで「SSL ハンドシェイク失敗」や証明書エラーが出る**
   - `MGY_TLS_CERT` にリーフ証明書単体（`cert.pem`）ではなく、**中間証明書を含むフルチェーン（`fullchain.pem`）**を指定しているか確認してください；
   - 入力したドメイン名が証明書の SAN/CN と一致しているか、`https://` と TLS ポート（`58943` など、HTTP `58900` ではない）を指定しているか確認してください。
3. **「パブリックアドレスでは平文 HTTP は利用できません」と表示される**
   - セキュリティ仕様（iOS ATS および Android `LanCleartextSecurityInterceptor`）により、グローバルアドレスへの平文 HTTP は禁止されています。方式 1 または方式 2 で HTTPS を構成し、`https://` で接続してください。
4. **なぜ `-tls-port` を `-port` と同じ番号（例：どちらも `58900`）にできないのですか？**
   - HTTPS 専用ポートを分離することで、ローカル CLI（`mgy pair` / `list` / `clear`、`mgy service status`）や LAN Wi-Fi の HTTP 接続を一切妨げず、ルーターの IPv6 ファイアウォールでも HTTPS ポートだけを安全に開放できるようにするためです。
5. **`-ipv6` を有効にすると、従来の IPv4 LAN や `127.0.0.1` に繋げなくなりますか？**
   - いいえ。`-host ""` または `-host ::` の場合は標準の **IPv4/IPv6 デュアルスタックソケット**として待ち受けるため、`127.0.0.1`、`192.168.x.x`、IPv6 のすべてが同時に利用できます。
6. **IPv6 が使えない社内 Wi-Fi では切断されてしまいますか？**
   - `CF_TUNNEL_ENABLED=0` にしない限り、カスタム IPv6 アドレスに到達できない場合は自動的に公式 E2EE リレーへ切り替わり、5G や IPv6 対応環境に戻ると再び直結へ復帰します。

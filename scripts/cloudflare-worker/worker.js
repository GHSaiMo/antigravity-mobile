/**
 * Multigravity Cloudflare Tunnel Dispatcher (Serverless Worker)
 * 
 * 架构：方案 A - Cloudflare 原生隧道与 DNS 自动化联动（客户端智能 Anycast 优选加速）
 * 作用：为 Multigravity 网关客户端自动按机器指纹创建、检索专属永久 Cloudflare Tunnel。
 * 特性：
 *   - 自动在 Zone DNS 中配置专属 CNAME 记录并开启 Proxied (橙色云朵)；
 *   - 自动生成 Ingress 规则并提供本地网关全量兜底，绝不 404；
 *   - 完美适配根域通配符 Universal SSL (*.jiuge.space)，秒级自动覆盖，无需证书等待；
 *   - 无需 Cloudflare for SaaS，无需第三方外部 DNS，彻底根除 Error 1000 自环；
 *   - 原生支持手机客户端通过亚太 Anycast 优选节点（35~50ms）免 DNS 污染极速直连。
 * 部署环境：Cloudflare Workers (免费版即可，每日 100,000 次请求额度)
 * 
 * 必需环境变量 / Secrets (在 Cloudflare Worker 设置页面配置):
 *   - CF_ACCOUNT_ID: Cloudflare 账户 ID (在控制台右侧可查)
 *   - CF_API_TOKEN:  Cloudflare API Token (需包含 Account: Cloudflare Tunnel: Edit 与 Zone: DNS: Edit 权限)
 *   - CF_ZONE_ID:    域名的 Zone ID
 *   - BASE_DOMAIN:   基础域名 (推荐根域名 jiuge.space)
 * 
 * 可选配置:
 *   - INVITE_CODE:   群专属接入暗号 (留空则全公开无感接入)
 *   - TARGET_PORT:   本地目标端口 (默认 58900，严格锁定禁止代理其他端口)
 */

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    // 允许跨域预检
    if (request.method === "OPTIONS") {
      return new Response(null, {
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
          "Access-Control-Allow-Headers": "Content-Type, Authorization, X-Invite-Code",
        },
      });
    }

    // 基础健康检查
    if (url.pathname === "/" || url.pathname === "/healthz") {
      return jsonResponse({
        status: "ok",
        service: "Multigravity Cloudflare Tunnel Dispatcher",
        version: "1.3.0",
        architecture: "Scheme A (Native Tunnel + Anycast Edge Acceleration)",
        base_domain: env.BASE_DOMAIN || "jiuge.space"
      });
    }

    // 核心接口：按机器指纹注册 / 获取专属永久 Tunnel
    if (url.pathname === "/api/tunnel/register" && request.method === "POST") {
      return handleTunnelRegister(request, env);
    }

    return jsonResponse({ error: "Not Found" }, 404);
  },
};

async function handleTunnelRegister(request, env) {
  // 1. 验证 Worker 环境变量完整性（支持 CF_* 或 CLOUDFLARE_* 前缀）
  const CF_ACCOUNT_ID = (env.CF_ACCOUNT_ID || env.CLOUDFLARE_ACCOUNT_ID || "").trim();
  const CF_API_TOKEN = (env.CF_API_TOKEN || env.CLOUDFLARE_API_TOKEN || "").trim();
  const CF_ZONE_ID = (env.CF_ZONE_ID || env.CLOUDFLARE_ZONE_ID || "").trim();
  const BASE_DOMAIN = (env.BASE_DOMAIN || "jiuge.space").trim();
  if (!CF_ACCOUNT_ID || !CF_API_TOKEN || !CF_ZONE_ID || !BASE_DOMAIN) {
    return jsonResponse({
      error: "Worker environment misconfigured. Please check CF_ACCOUNT_ID, CF_API_TOKEN, CF_ZONE_ID, BASE_DOMAIN.",
    }, 500);
  }

  // 2. 解析请求体
  let body;
  try {
    body = await request.json();
  } catch (e) {
    return jsonResponse({ error: "Invalid JSON body" }, 400);
  }

  const machineId = (body.machine_id || "").trim();
  const platform = (body.platform || "unknown").trim();
  const inviteCode = (body.invite_code || request.headers.get("X-Invite-Code") || "").trim();

  if (!machineId) {
    return jsonResponse({ error: "machine_id is required" }, 400);
  }

  // 3. 邀请码校验 (若设置了 INVITE_CODE)
  if (env.INVITE_CODE && env.INVITE_CODE.trim() !== "") {
    if (inviteCode !== env.INVITE_CODE.trim()) {
      return jsonResponse({ error: "Invalid invite code" }, 403);
    }
  }

  // 4. 生成统一稳定的 Short ID (8位纯字母数字，全小写)
  const shortId = await generateStableShortId(machineId);
  const tunnelName = `mgy-${shortId}`;
  const subdomain = `${shortId}.${BASE_DOMAIN}`;
  const targetPort = env.TARGET_PORT || "58900";

  const cfHeaders = {
    "Authorization": `Bearer ${CF_API_TOKEN}`,
    "Content-Type": "application/json",
  };

  try {
    // 5. 检查是否已存在同名 Tunnel（幂等处理：同一台电脑永远只分配一个固定隧道）
    const searchRes = await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${CF_ACCOUNT_ID}/cfd_tunnel?name=${encodeURIComponent(tunnelName)}&is_deleted=false`,
      { headers: cfHeaders }
    );
    const searchData = await searchRes.json();

    if (searchData.success && searchData.result && searchData.result.length > 0) {
      const existingTunnel = searchData.result[0];
      const tunnelId = existingTunnel.id;

      // 同步刷新 Ingress 路由规则与 Cloudflare DNS 原生映射
      await Promise.allSettled([
        setTunnelConfigurations(CF_ACCOUNT_ID, tunnelId, subdomain, targetPort, cfHeaders),
        syncTunnelDns(CF_ZONE_ID, tunnelId, subdomain, cfHeaders),
      ]);

      // 获取该 Tunnel 的 run token
      const token = await fetchTunnelToken(CF_ACCOUNT_ID, tunnelId, cfHeaders);
      if (token) {
        return jsonResponse({
          success: true,
          reused: true,
          tunnel_id: tunnelId,
          tunnel_name: tunnelName,
          subdomain: subdomain,
          url: `https://${subdomain}`,
          token: token,
        });
      }
    }

    // 6. 不存在时，新建 Tunnel
    // 生成 32 字节高熵随机 Secret
    const rawSecret = new Uint8Array(32);
    crypto.getRandomValues(rawSecret);
    const tunnelSecretBase64 = btoa(String.fromCharCode(...rawSecret));

    const createRes = await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${CF_ACCOUNT_ID}/cfd_tunnel`,
      {
        method: "POST",
        headers: cfHeaders,
        body: JSON.stringify({
          name: tunnelName,
          tunnel_secret: tunnelSecretBase64,
        }),
      }
    );
    const createData = await createRes.json();
    if (!createData.success || !createData.result) {
      return jsonResponse({
        error: "Failed to create Cloudflare Tunnel",
        details: createData.errors,
      }, 502);
    }
    const tunnelId = createData.result.id;

    // 7 & 8. 并行配置 Ingress 规则与 Cloudflare DNS 映射
    await Promise.allSettled([
      setTunnelConfigurations(CF_ACCOUNT_ID, tunnelId, subdomain, targetPort, cfHeaders),
      syncTunnelDns(CF_ZONE_ID, tunnelId, subdomain, cfHeaders),
    ]);

    // 9. 获取刚刚创建的 Tunnel 的运行 Token
    const token = await fetchTunnelToken(CF_ACCOUNT_ID, tunnelId, cfHeaders);
    if (!token) {
      return jsonResponse({
        error: "Tunnel created but failed to retrieve run token",
      }, 502);
    }

    return jsonResponse({
      success: true,
      reused: false,
      tunnel_id: tunnelId,
      tunnel_name: tunnelName,
      subdomain: subdomain,
      url: `https://${subdomain}`,
      token: token,
    });
  } catch (err) {
    return jsonResponse({
      error: "Internal error processing tunnel dispatch",
      message: err.message,
    }, 500);
  }
}

/**
 * 设置或刷新 Cloudflare Tunnel 内部 Ingress 规则
 * 精准绑定客户端专属子域名，并在末尾全量兜底直达本地网关，绝不返回 404
 */
async function setTunnelConfigurations(accountId, tunnelId, subdomain, targetPort, headers) {
  const res = await fetch(
    `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}/configurations`,
    {
      method: "PUT",
      headers,
      body: JSON.stringify({
        config: {
          ingress: [
            {
              hostname: subdomain,
              service: `http://localhost:${targetPort}`,
            },
            {
              service: `http://localhost:${targetPort}`, // 关键：全量兜底直达网关，绝不 404！
            },
          ],
        },
      }),
    }
  );
  return res.json();
}

/**
 * 为专属子域名创建/更新原生的 Cloudflare DNS CNAME 记录，开启 Proxy (橙色云朵)
 * 1. 彻底解除外部 DNS 与 SaaS 回退源依赖；
 * 2. 直连对应机器的 Argo Tunnel (${tunnelId}.cfargotunnel.com)；
 * 3. 根域通配符证书 (*.jiuge.space) 瞬时保护。
 */
async function syncTunnelDns(zoneId, tunnelId, subdomain, headers) {
  const dnsTarget = `${tunnelId}.cfargotunnel.com`;

  try {
    const dnsSearchRes = await fetch(
      `https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records?name=${encodeURIComponent(subdomain)}`,
      { headers }
    );
    const dnsSearchData = await dnsSearchRes.json();

    if (dnsSearchData.success && dnsSearchData.result && dnsSearchData.result.length > 0) {
      const existing = dnsSearchData.result[0];
      if (existing.type !== "CNAME" || existing.content !== dnsTarget || !existing.proxied) {
        await fetch(`https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records/${existing.id}`, {
          method: "PUT",
          headers,
          body: JSON.stringify({
            type: "CNAME",
            name: subdomain,
            content: dnsTarget,
            proxied: true,
            ttl: 1,
          }),
        });
      }
    } else {
      await fetch(`https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          type: "CNAME",
          name: subdomain,
          content: dnsTarget,
          proxied: true,
          ttl: 1,
        }),
      });
    }
  } catch (e) {
    console.error("Failed to sync tunnel DNS record:", e);
  }
}

async function fetchTunnelToken(accountId, tunnelId, headers) {
  const tokenRes = await fetch(
    `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}/token`,
    { headers }
  );
  const tokenData = await tokenRes.json();
  if (tokenData.success && tokenData.result) {
    return tokenData.result;
  }
  return null;
}

async function generateStableShortId(input) {
  const msgUint8 = new TextEncoder().encode(input.toLowerCase().trim());
  const hashBuffer = await crypto.subtle.digest("SHA-256", msgUint8);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  // 转换成十六进制取前 8 位
  const hex = hashArray.map(b => b.toString(16).padStart(2, "0")).join("");
  return hex.substring(0, 8);
}

function jsonResponse(data, status = 200) {
  return new Response(JSON.stringify(data, null, 2), {
    status,
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Access-Control-Allow-Origin": "*",
    },
  });
}

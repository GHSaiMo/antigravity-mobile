import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const mediaSource = fs.readFileSync(path.join(__dirname, "js", "media.js"), "utf8");

function loadMediaRuntime() {
  const ctx = {
    window: {
      location: { origin: "http://localhost:8080" },
      matchMedia: () => ({ matches: false }),
    },
    location: { origin: "http://localhost:8080", host: "localhost:8080" },
    document: {},
    fetch: () => Promise.resolve({ json: () => Promise.resolve({}) }),
    URL,
    URLSearchParams,
    console,
    setTimeout,
    Map,
    Set,
    Object,
    Array,
    String,
    Number,
    Boolean,
    RegExp,
    Math,
    Date,
    decodeURIComponent,
    encodeURIComponent,
  };
  vm.runInNewContext(mediaSource, ctx, { filename: "web/js/media.js" });
  return ctx;
}

const media = loadMediaRuntime();

function assertIntactImageCard(html) {
  assert.match(html, /<div class="image-thumb-card" data-action="open-lightbox"/);
  assert.doesNotMatch(html, /<div class="image-thumb-card"[^>]*(?:<br\/?>|\n)/);
  assert.doesNotMatch(html, /<img [^>]*(?:<br\/?>|\n)/);
  assert.match(html, /class="image-thumb-img" loading="lazy" onload="handleThumbnailLoad\(this\)" onerror="handleThumbnailError\(this\)" \/>/);
  assert.doesNotMatch(html, /<div class="image-thumb-card"[\s\S]*?<br\/>[\s\S]*?<\/div>/);
}

test("standalone markdown image renders as clean block without <br/> or attribute overflow", () => {
  const html = media.renderMarkdown("![示例图片](/path/to/demo.jpg)");
  assert.ok(html.startsWith('<div class="image-thumb-card" '), `expected standalone block, got: ${html.slice(0, 80)}`);
  assert.ok(!html.includes("<p>"), "standalone markdown image should not be wrapped in <p>");
  assert.ok(!html.includes("\n"), "thumbnail card HTML should contain no raw newlines");
  assertIntactImageCard(html);
  assert.match(html, /data-original-url="\/api\/v1\/files\/raw\?uri&#61;%2Fpath%2Fto%2Fdemo\.jpg&amp;preview&#61;1"/);
  assert.match(html, /data-title="demo\.jpg"/);
  assert.match(html, /data-alt="示例图片"/);
});

test("inline, blockquote, alert, and linked markdown images stay intact after newline-to-br conversion", () => {
  const cases = [
    "第一行说明\n行内配图：![示例图片](/path/to/demo.jpg) 结束\n第三行",
    "> 引用说明\n> ![引用图片](/path/to/quote.png)",
    "> [!NOTE]\n> 提示内容\n> ![提示图片](/path/to/alert.png)",
    "查看：[![说明](/path/to/thumb.jpg)](/path/to/orig.jpg) 详情",
    "直链图片：[截图说明](/path/to/shot.png)",
  ];
  for (const md of cases) {
    const html = media.renderMarkdown(md);
    assertIntactImageCard(html);
  }

  const paraHtml = media.renderMarkdown("第一行\n见图 ![示例](/path/to/demo.jpg)\n第三行");
  assert.match(paraHtml, /^<p>第一行<br\/>见图 <div class="image-thumb-card" /);
  assert.match(paraHtml, /<\/div><br\/>第三行<\/p>$/);
});

test("standalone image line terminates preceding paragraph without invalid nesting", () => {
  const html = media.renderMarkdown("这是说明文字\n![截图](/path/to/shot.png)\n后续说明");
  assert.match(html, /^<p>这是说明文字<\/p><div class="image-thumb-card" /);
  assert.match(html, /<\/div><p>后续说明<\/p>$/);
});

test("inline code protects markdown image, link, MEDIA:, agent-embed, and auto-link syntax", () => {
  const imgCode = media.renderMarkdown("独立段落的 `![说明](路径)`");
  assert.equal(imgCode, '<p>独立段落的 <code class="inline-code">![说明](路径)</code></p>');

  const tableCode = media.renderMarkdown("| 示例 | 效果 |\n| --- | --- |\n| `![说明](路径)` | 描述 |");
  assert.ok(!tableCode.includes("image-thumb-card"), "inline code inside table must not expand to image card");
  assert.ok(tableCode.includes('<td><code class="inline-code">![说明](路径)</code></td>'));

  const linkCode = media.renderMarkdown("示例 `[foo](https://example.com)`");
  assert.equal(linkCode, '<p>示例 <code class="inline-code">[foo](https://example.com)</code></p>');

  const mediaCode = media.renderMarkdown("示例 `MEDIA:/path/to/a.png` 和 `foo MEDIA:/path/to/b.png`");
  assert.equal(
    mediaCode,
    '<p>示例 <code class="inline-code">MEDIA:/path/to/a.png</code> 和 <code class="inline-code">foo MEDIA:/path/to/b.png</code></p>'
  );

  const embedCode = media.renderMarkdown('用法：`<agent-embed src="foo.html"></agent-embed>`');
  assert.equal(
    embedCode,
    '<p>用法：<code class="inline-code">&lt;agent-embed src=&quot;foo.html&quot;&gt;&lt;/agent-embed&gt;</code></p>'
  );

  const planCode = media.renderMarkdown("查看 `implementation_plan.md` 与 `walkthrough.md`");
  assert.equal(
    planCode,
    '<p>查看 <code class="inline-code">implementation_plan.md</code> 与 <code class="inline-code">walkthrough.md</code></p>'
  );

  const doubleTick = media.renderMarkdown("双反引号 `` `![x](y)` ``");
  assert.equal(doubleTick, '<p>双反引号 <code class="inline-code">`![x](y)`</code></p>');
});

test("links and images whose label contains inline code preserve valid HTML attributes", () => {
  const mdLink = media.renderMarkdown("[`notes.md`](/path/to/notes.md)");
  assert.match(mdLink, /data-md-title="notes\.md"/);
  assert.ok(!mdLink.includes('data-md-title="<code'), "data-md-title must not contain raw HTML tags");
  assert.match(mdLink, /\/icons\/files\/markdown\.svg/);
  assert.match(mdLink, /<span><code class="inline-code">notes\.md<\/code><\/span>/);

  const extLink = media.renderMarkdown("[`click`](https://example.com)");
  assert.equal(
    extLink,
    '<p><a href="https://example.com" class="text-link" target="_blank" rel="noopener noreferrer"><span><code class="inline-code">click</code></span></a></p>'
  );

  const imgWithCodeAlt = media.renderMarkdown("文本 ![`demo`](/path/to/demo.png)");
  assertIntactImageCard(imgWithCodeAlt);
  assert.match(imgWithCodeAlt, /data-alt="demo"/);
  assert.match(imgWithCodeAlt, /alt="demo"/);

  const imgLinkWithCodeLabel = media.renderMarkdown("文本 [`pic`](/path/to/a.png)");
  assertIntactImageCard(imgLinkWithCodeLabel);
  assert.match(imgLinkWithCodeLabel, /data-alt="pic"/);
});

test("image URLs with & query parameters and special alt characters are not double-escaped", () => {
  const inlineHtml = media.renderMarkdown('见图 ![chart & "q"](https://example.com/img.png?a=1&b=2)');
  assertIntactImageCard(inlineHtml);
  assert.match(inlineHtml, /data-original-url="https:\/\/example\.com\/img\.png\?a&#61;1&amp;b&#61;2"/);
  assert.ok(!inlineHtml.includes("&amp;amp;"), "URL ampersand must not be double-escaped");
  assert.match(inlineHtml, /data-alt="chart &amp; &quot;q&quot;"/);

  const standaloneHtml = media.renderMarkdown('![chart & "q"](https://example.com/img.png?a=1&b=2)');
  assertIntactImageCard(standaloneHtml);
  assert.match(standaloneHtml, /data-original-url="https:\/\/example\.com\/img\.png\?a&#61;1&amp;b&#61;2"/);
  assert.ok(!standaloneHtml.includes("&amp;amp;"));
});

test("image filenames containing markdown emphasis/strike/highlight markers are not mutated", () => {
  const html = media.renderMarkdown(
    "测试 ![图](/path/to/a*b*c.png) 和 ![图](/path/to/a~~b~~.png) 和 ![图](/path/to/a==b==.png)"
  );
  assert.ok(html.includes('>a*b*c.png</span>'), "asterisk in filename must not become <em>");
  assert.ok(html.includes('>a~~b~~.png</span>'), "tildes in filename must not become <del>");
  assert.ok(html.includes('>a==b==.png</span>'), "equals in filename must not become <mark>");
});

test("inline MEDIA: preserves leading separator and inline <agent-embed> renders compact card", () => {
  const mediaHtml = media.renderMarkdown("请看 MEDIA:/path/to/a.png");
  assert.match(mediaHtml, /^<p>请看 <div class="image-thumb-card" /);
  assertIntactImageCard(mediaHtml);

  const embedHtml = media.renderMarkdown('正文 <agent-embed src="/tmp/a.html?x=1&y=2"></agent-embed> 结束');
  assert.match(embedHtml, /^<p>正文 <div class="agent-embed-card" /);
  assert.match(embedHtml, /data-src="\/tmp\/a\.html\?x&#61;1&amp;y&#61;2"/);
  assert.ok(!embedHtml.includes("&amp;amp;"));
  assert.ok(!embedHtml.includes("<br/>"), "agent-embed card must not have <br/> injected");

  const singleQuoteEmbed = media.renderMarkdown("正文 <agent-embed src='/tmp/b.html'></agent-embed> 结束");
  assert.match(singleQuoteEmbed, /<div class="agent-embed-card" data-src="\/tmp\/b\.html"/);
});

test("lone unclosed backtick does not swallow subsequent markdown image", () => {
  const html = media.renderMarkdown("It's a ` lone backtick ![图](/path/to/demo.jpg)");
  assertIntactImageCard(html);
  assert.ok(html.includes("It&#039;s a ` lone backtick "));
});

test("repairMermaid quotes bare subgraph titles and nested-paren node labels without touching plain IDs", () => {
  const raw = [
    "flowchart TD",
    "  subgraph Container 1: 40ft FCL (4,000 Gallons)",
    "    A(LT Primer (Base Paint)) --> B[Alcro Primer (4,000 Gal)]",
    "  end",
    "  subgraph backend",
    "    C((Circle Node)) --> D([Stadium])",
    "  end",
    '  subgraph "Already Quoted (OK)"',
    "    E[Plain]",
    "  end",
  ].join("\n");

  const repaired = media.repairMermaid(raw);
  assert.ok(
    repaired.includes('  subgraph "Container 1: 40ft FCL (4,000 Gallons)"'),
    `expected bare subgraph title to be quoted, got:\n${repaired}`
  );
  assert.ok(
    repaired.includes('A("LT Primer (Base Paint)")'),
    `expected nested-paren node label to be quoted, got:\n${repaired}`
  );
  assert.ok(
    repaired.includes('B["Alcro Primer (4,000 Gal)"]'),
    `expected square-bracket label with parens to be quoted, got:\n${repaired}`
  );
  assert.ok(
    repaired.includes("  subgraph backend\n"),
    `expected plain single-word subgraph ID to stay unquoted, got:\n${repaired}`
  );
  assert.ok(
    repaired.includes("C((Circle Node)) --> D([Stadium])"),
    `expected mermaid circle/stadium shapes to stay intact, got:\n${repaired}`
  );
  assert.ok(
    repaired.includes('  subgraph "Already Quoted (OK)"'),
    `expected already-quoted subgraph to stay intact, got:\n${repaired}`
  );
});

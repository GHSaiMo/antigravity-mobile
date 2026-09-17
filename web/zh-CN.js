/**
 * Antigravity Desktop Web Workbench - 精准 UI 中文化引擎 (zh-CN)
 * 仅汉化操作界面、侧边栏、状态胶囊、按钮及提示词，严禁篡改代码块、命令参数与消息正文。
 */
(function () {
  'use strict';

  // 1. 静态短语字典 (精确匹配 / 去除首尾空格后匹配)
  const exactDict = new Map([
    // 侧边栏与主导航
    ['New Conversation', '新建会话'],
    ['Conversation History', '历史会话'],
    ['Scheduled Tasks', '定时任务'],
    ['Projects', '工程项目'],
    ['Settings', '系统设置'],
    ['Untitled Conversation', '未命名会话'],
    ['No conversations yet', '暂无历史会话'],
    ['Open Conversation History', '打开历史会话'],
    ['Open Scheduled Tasks', '打开定时任务'],
    ['Collapse sidebar', '收起侧边栏'],
    ['Expand sidebar', '展开侧边栏'],
    ['Close sidebar', '关闭侧边栏'],

    // 常用操作与按键
    ['Proceed', '确认执行 (Proceed)'],
    ['Always Proceed', '始终自动执行'],
    ['Always Ask', '每次询问确认'],
    ['Approve', '批准'],
    ['Reject', '拒绝'],
    ['Cancel', '取消'],
    ['Confirm', '确认'],
    ['Save', '保存'],
    ['Delete', '删除'],
    ['Rename', '重命名'],
    ['Retry', '重试'],
    ['Copy', '复制'],
    ['Copied!', '已复制!'],
    ['Local', '本地执行'],
    ['Cloud', '云端执行'],
    ['Commit and Push', '提交并推送 (Git)'],

    // 状态与执行提示
    ['Working..', '智能体处理中...'],
    ['Working...', '智能体处理中...'],
    ['Thinking...', '思考中...'],
    ['Generating...', '正在生成...'],
    ['Completed', '已完成'],
    ['Failed', '执行失败'],
    ['Interrupted', '已中断'],
    ['Stopped', '已停止'],

    // 设置与筛选
    ['General', '常规'],
    ['Application', '应用程序'],
    ['Appearance', '外观主题'],
    ['Theme', '主题'],
    ['Dark', '深色'],
    ['Light', '浅色'],
    ['System', '跟随系统'],
    ['Model Selection', '模型选择'],
    ['Filter', '筛选'],
    ['Search', '搜索'],
    ['Clear', '清除'],
    ['All', '全部']
  ]);

  // 2. 动态正则匹配规则
  const regexRules = [
    [/^Ask anything, @ to mention, \/ for actions$/i, '输入任何问题，输入 @ 引用，输入 / 触发动作...'],
    [/^Ask anything, @ to mention$/i, '输入任何问题，输入 @ 引用文件...'],
    [/^Ran (\d+) commands?$/i, '已执行 $1 条指令'],
    [/^Explored (\d+) files?, (\d+) folders?$/i, '已探索 $1 个文件，$2 个目录'],
    [/^Explored (\d+) files?, (\d+) search(?:es)?$/i, '已探索 $1 个文件，$2 次检索'],
    [/^Explored (\d+) files?$/i, '已探索 $1 个文件'],
    [/^Explored (\d+) folders?$/i, '已探索 $1 个目录'],
    [/^Thought for (\d+)s?$/i, '深度思考 $1 秒'],
    [/^Thought for (\d+)m (\d+)s?$/i, '深度思考 $1 分 $2 秒'],
    [/^Running (\d+) commands?$/i, '正在运行 $1 条指令...'],
    [/^(\d+) steps?$/i, '$1 个步骤'],
    [/^(\d+) conversations?$/i, '$1 个会话'],
    [/^(\d+)m ago$/i, '$1 分钟前'],
    [/^(\d+)h ago$/i, '$1 小时前'],
    [/^(\d+)d ago$/i, '$1 天前'],
    [/^just now$/i, '刚刚']
  ];

  // 3. 安全检测：判断是否为不可汉化的代码块或数据区域
  const IGNORE_TAGS = new Set(['SCRIPT', 'STYLE', 'CODE', 'PRE', 'NOSCRIPT', 'SVG', 'PATH']);

  function shouldIgnoreElement(el) {
    if (!el || !el.tagName) return true;
    if (IGNORE_TAGS.has(el.tagName)) return true;

    // 排除编辑器、终端、代码高亮容器
    const className = typeof el.className === 'string' ? el.className : '';
    if (
      className.includes('monaco-editor') ||
      className.includes('syntax-highlight') ||
      className.includes('xterm') ||
      className.includes('terminal') ||
      className.includes('code-block') ||
      className.includes('language-')
    ) {
      return true;
    }

    // 排除可编辑区本身的内容文本（但允许汉化 placeholder）
    if (el.isContentEditable) return true;

    return false;
  }

  // 4. 单一文本翻译
  function translateString(str) {
    if (!str || typeof str !== 'string') return str;
    const trimmed = str.trim();
    if (!trimmed) return str;

    // 优先静态查表
    if (exactDict.has(trimmed)) {
      const translated = exactDict.get(trimmed);
      return str.replace(trimmed, translated);
    }

    // 正则动态查表
    for (let i = 0; i < regexRules.length; i++) {
      const [re, repl] = regexRules[i];
      if (re.test(trimmed)) {
        const translated = trimmed.replace(re, repl);
        return str.replace(trimmed, translated);
      }
    }

    return str;
  }

  // 5. 遍历并翻译节点
  const processedNodes = new WeakSet();

  function translateNode(node) {
    if (!node) return;

    if (node.nodeType === Node.TEXT_NODE) {
      if (processedNodes.has(node)) return;
      const parent = node.parentElement;
      if (parent && shouldIgnoreElement(parent)) return;

      const original = node.nodeValue;
      if (!original || !original.trim()) return;

      const translated = translateString(original);
      if (translated !== original) {
        processedNodes.add(node);
        node.nodeValue = translated;
      }
      return;
    }

    if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node;
      if (shouldIgnoreElement(el)) return;

      // 翻译 input / textarea 的 placeholder
      if (el.placeholder) {
        const trPlaceholder = translateString(el.placeholder);
        if (trPlaceholder !== el.placeholder) {
          el.placeholder = trPlaceholder;
        }
      }

      // 翻译 aria-label 或 title 提示
      if (el.getAttribute('aria-label')) {
        const label = el.getAttribute('aria-label');
        const trLabel = translateString(label);
        if (trLabel !== label) {
          el.setAttribute('aria-label', trLabel);
        }
      }
      if (el.title) {
        const trTitle = translateString(el.title);
        if (trTitle !== el.title) {
          el.title = trTitle;
        }
      }

      // 递归子节点
      for (let child = el.firstChild; child; child = child.nextSibling) {
        translateNode(child);
      }
    }
  }

  // 6. 初始扫描与全量 DOM 监听
  function runLocalization() {
    if (document.body) {
      translateNode(document.body);
    }
  }

  // 监听动态 DOM 变动
  let timer = null;
  const observer = new MutationObserver((mutations) => {
    // 节流处理，提升高频打字机输出时的 UI 渲染性能
    if (timer) return;
    timer = requestAnimationFrame(() => {
      timer = null;
      for (let i = 0; i < mutations.length; i++) {
        const m = mutations[i];
        if (m.type === 'childList') {
          for (let j = 0; j < m.addedNodes.length; j++) {
            translateNode(m.addedNodes[j]);
          }
        } else if (m.type === 'characterData') {
          translateNode(m.target);
        }
      }
    });
  });

  // 启动观察器
  function init() {
    runLocalization();
    if (document.body) {
      observer.observe(document.body, {
        childList: true,
        subtree: true,
        characterData: true
      });
    } else {
      document.addEventListener('DOMContentLoaded', () => {
        runLocalization();
        observer.observe(document.body, {
          childList: true,
          subtree: true,
          characterData: true
        });
      });
    }
    // 备用定时扫描兜底（处理某些 React 异步重渲染）
    setInterval(runLocalization, 1500);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();

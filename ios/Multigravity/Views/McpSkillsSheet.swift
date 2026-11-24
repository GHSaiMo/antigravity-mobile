import SwiftUI

public struct McpSkillsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    
    @State private var selectedTab: Tab = .mcp
    @State private var searchText: String = ""
    @State private var isLoading: Bool = false
    @State private var errorMessage: String? = nil
    
    @State private var mcpServers: [McpServerState] = []
    @State private var skills: [SkillInfo] = []
    @State private var plugins: [CascadePluginItem] = []
    @State private var pluginProviders: [PluginAuthProviderItem] = []
    @State private var pluginPermissionGrants: [PluginPermissionGrantItem] = []
    @State private var revokingGrantIds: Set<String> = []
    @State private var automations: [AutomationSidecarItem] = []
    @State private var automationEvents: [AutomationEventItem] = []
    @State private var backgroundTasks: [GlobalBackgroundTaskItem] = []
    
    @State private var expandedServerNames: Set<String> = []
    @State private var expandedToolIds: Set<String> = []
    @State private var viewingSkill: SkillInfo? = nil
    @State private var installingPluginIds: Set<String> = []
    @State private var managingAutomationNames: Set<String> = []
    
    @State private var expandedLogTaskIds: Set<String> = []
    @State private var taskLogs: [String: String] = [:]
    @State private var loadingLogTaskIds: Set<String> = []
    @State private var stoppingTaskIds: Set<String> = []
    @State private var copiedTaskId: String? = nil

    // Phase 3 State (MCP toggle, Agent Memories CRUD, Automation CRUD)
    @State private var togglingMcpNames: Set<String> = []
    @State private var memories: [MemoryRuleItem] = []
    @State private var showMemoryEditor: Bool = false
    @State private var editingMemory: MemoryRuleItem? = nil
    @State private var pendingDeleteMemory: MemoryRuleItem? = nil
    @State private var memScope: String = "memory"
    @State private var memTitle: String = ""
    @State private var memContent: String = ""
    @State private var isSavingMemory: Bool = false
    @State private var showCreateAutomation: Bool = false
    @State private var newAutoName: String = ""
    @State private var newAutoSchedule: String = "0 9 * * *"
    @State private var newAutoPrompt: String = ""
    @State private var isCreatingAutomation: Bool = false
    @State private var pendingDeleteAutomation: AutomationSidecarItem? = nil
    @State private var selectedAutomationId: String? = nil
    @State private var showStopAllTasksConfirm: Bool = false
    @State private var pendingStopDaemonTask: GlobalBackgroundTaskItem? = nil
    @State private var actionErrorMessage: String? = nil
    
    private let currentConversationId: String?
    private let hasActiveSwarm: Bool
    private let onSelectConversation: ((String) -> Void)?
    private let onTasksChanged: (([GlobalBackgroundTaskItem]) -> Void)?
    private let onOpenSwarm: (() -> Void)?
    
    public enum Tab: String, CaseIterable, Identifiable {
        case mcp = "MCP"
        case skills = "技能"
        case plugins = "插件"
        case automations = "定时"
        case tasks = "任务"
        public var id: String { rawValue }
    }
    
    public init(
        initialTab: Tab = .mcp,
        currentConversationId: String? = nil,
        hasActiveSwarm: Bool = false,
        onSelectConversation: ((String) -> Void)? = nil,
        onTasksChanged: (([GlobalBackgroundTaskItem]) -> Void)? = nil,
        onOpenSwarm: (() -> Void)? = nil
    ) {
        self._selectedTab = State(initialValue: initialTab)
        self._mcpServers = State(initialValue: CacheManager.shared.loadMcpServers())
        self._skills = State(initialValue: CacheManager.shared.loadSkills())
        self.currentConversationId = currentConversationId
        self.hasActiveSwarm = hasActiveSwarm
        self.onSelectConversation = onSelectConversation
        self.onTasksChanged = onTasksChanged
        self.onOpenSwarm = onOpenSwarm
    }
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Segmented control
                Picker("类别", selection: $selectedTab) {
                    Text("MCP").tag(Tab.mcp)
                    Text("技能").tag(Tab.skills)
                    Text("插件").tag(Tab.plugins)
                    Text("定时").tag(Tab.automations)
                    Text("任务").tag(Tab.tasks)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.top, 10)
                .padding(.bottom, 8)
                
                // Content
                if isLoading && mcpServers.isEmpty && skills.isEmpty && backgroundTasks.isEmpty {
                    Spacer()
                    ProgressView("正在检测已连接服务与后台任务...")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                    Spacer()
                } else if let error = errorMessage, mcpServers.isEmpty && skills.isEmpty && plugins.isEmpty && automations.isEmpty && backgroundTasks.isEmpty {
                    errorStateView(error)
                } else {
                    List {
                        switch selectedTab {
                        case .mcp:
                            mcpServersSection
                        case .skills:
                            skillsSection
                        case .plugins:
                            pluginsSection
                        case .automations:
                            automationsSection
                        case .tasks:
                            backgroundTasksSection
                        }
                    }
                    .listStyle(.insetGrouped)
                    .searchable(text: $searchText, prompt: searchPrompt)
                    .refreshable {
                        await loadData()
                    }
                }
            }
            .navigationTitle("工具与任务看板")
            .navigationBarTitleDisplayMode(.inline)
            .presentationDragIndicator(.visible)
            .navigationDestination(item: $selectedAutomationId) { sid in
                if let sidecar = automations.first(where: { $0.id == sid }) {
                    AutomationDetailView(
                        sidecar: sidecar,
                        allEvents: automationEvents,
                        onRefresh: {
                            await refreshAutomations()
                        },
                        onDeleted: {
                            selectedAutomationId = nil
                        },
                        onSelectConversation: { cid in
                            dismiss()
                            onSelectConversation?(cid)
                        }
                    )
                } else {
                    Text("任务不存在或已删除")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                }
            }
            .sheet(item: $viewingSkill) { skill in
                skillDetailSheet(skill)
            }
            .sheet(isPresented: $showMemoryEditor) {
                memoryEditorSheet
            }
            .sheet(isPresented: $showCreateAutomation) {
                createAutomationSheet
            }
            .alert(
                "确认删除记忆条目？",
                isPresented: Binding(
                    get: { pendingDeleteMemory != nil },
                    set: { if !$0 { pendingDeleteMemory = nil } }
                ),
                presenting: pendingDeleteMemory
            ) { item in
                Button("取消", role: .cancel) {
                    pendingDeleteMemory = nil
                }
                Button("删除", role: .destructive) {
                    deleteMemoryItem(item)
                    pendingDeleteMemory = nil
                }
            } message: { item in
                Text("确定要删除「\(item.title)」吗？这会同时删除电脑端对应的规则或记忆文件。")
            }
            .confirmationDialog(
                "确认删除定时任务？",
                isPresented: Binding(
                    get: { pendingDeleteAutomation != nil },
                    set: { if !$0 { pendingDeleteAutomation = nil } }
                ),
                titleVisibility: .visible,
                presenting: pendingDeleteAutomation
            ) { item in
                Button("删除", role: .destructive) {
                    deleteAutomationItem(item)
                    pendingDeleteAutomation = nil
                }
                Button("取消", role: .cancel) {
                    pendingDeleteAutomation = nil
                }
            } message: { item in
                Text("确定要删除定时任务「\(item.name)」吗？删除后将停止调度并移除该任务配置。")
            }
            .confirmationDialog(
                "停止全部后台任务？",
                isPresented: $showStopAllTasksConfirm,
                titleVisibility: .visible
            ) {
                Button("全部停止", role: .destructive) {
                    let list = filteredBackgroundTasks
                    Task { await stopAllBackgroundTasks(list) }
                }
                Button("取消", role: .cancel) {}
            } message: {
                Text("将终止当前列表中所有会话的运行命令与常驻守护进程，未保存的运行状态可能会中断。")
            }
            .confirmationDialog(
                "停止常驻服务？",
                isPresented: Binding(
                    get: { pendingStopDaemonTask != nil },
                    set: { if !$0 { pendingStopDaemonTask = nil } }
                ),
                titleVisibility: .visible,
                presenting: pendingStopDaemonTask
            ) { task in
                Button("停止服务", role: .destructive) {
                    Task { await stopSingleTask(task) }
                    pendingStopDaemonTask = nil
                }
                Button("取消", role: .cancel) {
                    pendingStopDaemonTask = nil
                }
            } message: { task in
                Text("「\(task.displayTitle)」为常驻守护任务，终止后对应本地服务或监听端口将下线。")
            }
            .alert("操作失败", isPresented: Binding(
                get: { actionErrorMessage != nil },
                set: { if !$0 { actionErrorMessage = nil } }
            )) {
                Button("确定", role: .cancel) { actionErrorMessage = nil }
            } message: {
                Text(actionErrorMessage ?? "")
            }
            .task {
                await loadData()
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
    }
    
    private var searchPrompt: String {
        switch selectedTab {
        case .mcp: return String(localized: "搜索 MCP 服务或工具名称")
        case .skills: return String(localized: "搜索技能名称或描述")
        case .plugins: return String(localized: "搜索插件名称或描述")
        case .automations: return String(localized: "搜索定时任务或表达式")
        case .tasks: return String(localized: "搜索命令、会话标题或工作区")
        }
    }
    
    // MARK: - MCP Servers Section
    
    private var filteredMcpServers: [McpServerState] {
        let q = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty { return mcpServers }
        return mcpServers.filter { server in
            server.spec.serverName.lowercased().contains(q) ||
            (server.serverInfo?.name?.lowercased().contains(q) == true) ||
            (server.instructions?.lowercased().contains(q) == true) ||
            (server.tools?.contains { tool in
                tool.name.lowercased().contains(q) || (tool.description?.lowercased().contains(q) == true)
            } == true)
        }
    }
    
    @ViewBuilder
    private var mcpServersSection: some View {
        if filteredMcpServers.isEmpty {
            Section {
                VStack(spacing: 8) {
                    Image(systemName: "magnifyingglass")
                        .font(.system(size: 32))
                        .foregroundColor(.secondary)
                    Text("未找到匹配的 MCP 服务或工具")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
            }
        } else {
            ForEach(filteredMcpServers) { server in
                Section {
                    // Server summary row
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(alignment: .center, spacing: 8) {
                            Text(server.spec.serverName)
                                .font(.system(size: 16, weight: .bold))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                            
                            if let ver = server.serverInfo?.version, !ver.isEmpty {
                                Text("v\(ver)")
                                    .font(.system(size: 11, design: .monospaced))
                                    .lineLimit(1)
                                    .fixedSize(horizontal: true, vertical: false)
                                    .padding(.horizontal, 5)
                                    .padding(.vertical, 1.5)
                                    .background(Color(.secondarySystemBackground))
                                    .clipShape(RoundedRectangle(cornerRadius: 4))
                                    .foregroundColor(.secondary)
                            }
                            
                            Spacer(minLength: 4)
                            
                            HStack(spacing: 4) {
                                Circle()
                                    .fill(server.isReady ? Color.green : Color.orange)
                                    .frame(width: 7, height: 7)
                                Text(server.statusTitle)
                                    .font(.system(size: 11.5, weight: .medium))
                                    .lineLimit(1)
                                    .foregroundColor(server.isReady ? .green : .orange)
                            }
                            .fixedSize(horizontal: true, vertical: false)
                            .layoutPriority(1)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 3)
                            .background(server.isReady ? Color.green.opacity(0.1) : Color.orange.opacity(0.1))
                            .clipShape(Capsule())

                            Toggle("", isOn: Binding(
                                get: { server.spec.disabled != true },
                                set: { nextEnabled in
                                    toggleMcp(server.spec.serverName, enabled: nextEnabled)
                                }
                            ))
                            .labelsHidden()
                            .scaleEffect(0.85)
                            .disabled(togglingMcpNames.contains(server.spec.serverName))
                        }
                        
                        if let instructions = server.instructions, !instructions.isEmpty {
                            Text(instructions)
                                .font(.system(size: 13))
                                .foregroundColor(.secondary)
                                .lineLimit(2)
                        }
                        
                        if let cmd = server.spec.command, !cmd.isEmpty {
                            Text(cmd.split(separator: "/").last.map(String.init) ?? cmd)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(Color(.tertiaryLabel))
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }
                    .padding(.vertical, 4)
                    
                    // Tools list
                    if let tools = server.tools, !tools.isEmpty {
                        DisclosureGroup(
                            isExpanded: Binding(
                                get: { expandedServerNames.contains(server.spec.serverName) || !searchText.isEmpty },
                                set: { expanded in
                                    if expanded {
                                        expandedServerNames.insert(server.spec.serverName)
                                    } else {
                                        expandedServerNames.remove(server.spec.serverName)
                                    }
                                }
                            )
                        ) {
                            ForEach(tools) { tool in
                                toolItemView(tool)
                            }
                        } label: {
                            HStack(spacing: 6) {
                                Image(systemName: "wrench.and.screwdriver.fill")
                                    .font(.system(size: 13))
                                    .foregroundColor(.indigo)
                                Text("可用工具清单 (\(tools.count))")
                                    .font(.system(size: 14, weight: .medium))
                                    .foregroundColor(.indigo)
                            }
                            .padding(.vertical, 2)
                        }
                    } else {
                        Text("暂无注册工具")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                    }
                }
            }
        }
    }
    
    // MARK: - Tool Item View
    
    private func toolItemView(_ tool: McpToolDefinition) -> some View {
        let isExpanded = expandedToolIds.contains(tool.id)
        let params = tool.parsedParameters
        
        return VStack(alignment: .leading, spacing: 6) {
            Button(action: {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                if isExpanded {
                    expandedToolIds.remove(tool.id)
                } else {
                    expandedToolIds.insert(tool.id)
                }
            }) {
                HStack(alignment: .top, spacing: 8) {
                    VStack(alignment: .leading, spacing: 3) {
                        HStack(spacing: 6) {
                            Text(tool.name)
                                .font(.system(size: 13.5, weight: .semibold, design: .monospaced))
                                .foregroundColor(.primary)
                            
                            if !params.isEmpty {
                                Text("\(params.count) 参数")
                                    .font(.system(size: 10, weight: .medium))
                                    .padding(.horizontal, 5)
                                    .padding(.vertical, 1)
                                    .background(Color.indigo.opacity(0.1))
                                    .foregroundColor(.indigo)
                                    .clipShape(Capsule())
                            }
                        }
                        
                        if let desc = tool.description, !desc.isEmpty {
                            Text(desc)
                                .font(.system(size: 12.5))
                                .foregroundColor(.secondary)
                                .lineLimit(isExpanded ? nil : 2)
                        }
                    }
                    
                    Spacer()
                    
                    Image(systemName: isExpanded ? "chevron.up" : "chevron.down")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(Color(.tertiaryLabel))
                        .padding(.top, 4)
                }
            }
            .buttonStyle(.plain)
            
            // Expanded Parameters List
            if isExpanded {
                if params.isEmpty {
                    Text("无输入参数（直接调用）")
                        .font(.system(size: 12))
                        .foregroundColor(Color(.tertiaryLabel))
                        .padding(.leading, 8)
                        .padding(.vertical, 4)
                } else {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("参数定义：")
                            .font(.system(size: 11.5, weight: .semibold))
                            .foregroundColor(.secondary)
                        
                        ForEach(params, id: \.name) { param in
                            HStack(alignment: .top, spacing: 6) {
                                Text(param.name)
                                    .font(.system(size: 12, weight: .medium, design: .monospaced))
                                    .foregroundColor(.primary)
                                
                                if param.isRequired {
                                    Text("必填")
                                        .font(.system(size: 9.5, weight: .bold))
                                        .padding(.horizontal, 4)
                                        .padding(.vertical, 0.5)
                                        .background(Color.red.opacity(0.12))
                                        .foregroundColor(.red)
                                        .clipShape(RoundedRectangle(cornerRadius: 3))
                                }
                                
                                Text("(\(param.type))")
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundColor(.secondary)
                                
                                Spacer()
                                
                                if let def = param.defaultValue {
                                    Text("默认: \(def)")
                                        .font(.system(size: 10.5))
                                        .foregroundColor(Color(.tertiaryLabel))
                                }
                            }
                            
                            if !param.description.isEmpty {
                                Text(param.description)
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.secondary)
                                    .padding(.leading, 8)
                            }
                            
                            Divider()
                                .opacity(0.5)
                        }
                    }
                    .padding(8)
                    .background(Color(.secondarySystemBackground).opacity(0.6))
                    .clipShape(RoundedRectangle(cornerRadius: 8))
                }
            }
        }
        .padding(.vertical, 4)
    }
    
    // MARK: - Skills Section
    
    private var filteredSkills: [SkillInfo] {
        let q = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty { return skills }
        return skills.filter {
            $0.name.lowercased().contains(q) ||
            ($0.description?.lowercased().contains(q) == true)
        }
    }
    
    @ViewBuilder
    private var skillsSection: some View {
        Section {
            HStack {
                Label("Agent 记忆与规则库 (\(memories.count))", systemImage: "brain.head.profile")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.indigo)
                Spacer()
                Button("+ 添加") {
                    editingMemory = nil
                    memScope = "memory"
                    memTitle = ""
                    memContent = ""
                    showMemoryEditor = true
                }
                .font(.system(size: 12.5, weight: .semibold))
                .buttonStyle(.bordered)
                .tint(.indigo)
            }
            if memories.isEmpty {
                Text("暂无全局或记忆条目，可点击「+ 添加」创建")
                    .font(.system(size: 12.5))
                    .foregroundColor(.secondary)
            } else {
                ForEach(memories) { item in
                    VStack(alignment: .leading, spacing: 4) {
                        HStack(spacing: 6) {
                            Text(item.scope)
                                .font(.system(size: 10, weight: .semibold))
                                .padding(.horizontal, 5)
                                .padding(.vertical, 2)
                                .background(Color.indigo.opacity(0.12))
                                .foregroundColor(.indigo)
                                .clipShape(Capsule())
                            Text(item.title)
                                .font(.system(size: 13.5, weight: .semibold))
                                .lineLimit(1)
                            Spacer()
                            if !item.isReadOnly {
                                Button("编辑") {
                                    editingMemory = item
                                    memScope = item.scope
                                    memTitle = item.title
                                    memContent = item.content
                                    showMemoryEditor = true
                                }
                                .font(.system(size: 11.5))
                                .buttonStyle(.borderless)
                                Button("删除", role: .destructive) {
                                    pendingDeleteMemory = item
                                }
                                .font(.system(size: 11.5))
                                .buttonStyle(.borderless)
                            }
                        }
                        if !item.content.isEmpty {
                            Text(item.content)
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                                .lineLimit(3)
                        }
                    }
                    .padding(.vertical, 3)
                }
            }
        }

        if filteredSkills.isEmpty {
            Section {
                VStack(spacing: 8) {
                    Image(systemName: "sparkles")
                        .font(.system(size: 32))
                        .foregroundColor(.secondary)
                    Text("未找到匹配的技能")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
            }
        } else {
            ForEach(filteredSkills) { skill in
                Section {
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        viewingSkill = skill
                    }) {
                        VStack(alignment: .leading, spacing: 6) {
                            HStack(alignment: .center, spacing: 8) {
                                Text(skill.name)
                                    .font(.system(size: 15, weight: .bold))
                                    .foregroundColor(.primary)
                                
                                Text(skill.locationCategory)
                                    .font(.system(size: 10.5, weight: .medium))
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background(skill.isBuiltin ? Color.gray.opacity(0.12) : Color.indigo.opacity(0.12))
                                    .foregroundColor(skill.isBuiltin ? .secondary : .indigo)
                                    .clipShape(Capsule())
                                
                                Spacer()
                                
                                Image(systemName: "chevron.right")
                                    .font(.system(size: 12, weight: .semibold))
                                    .foregroundColor(Color(.tertiaryLabel))
                            }
                            
                            if let desc = skill.description, !desc.isEmpty {
                                Text(desc)
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                                    .lineLimit(3)
                            }
                            
                            if let path = skill.path, !path.isEmpty {
                                Text(path)
                                    .font(.system(size: 10.5, design: .monospaced))
                                    .foregroundColor(Color(.tertiaryLabel))
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                            }
                        }
                        .padding(.vertical, 4)
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }
    
    // MARK: - Skill Detail Sheet
    
    private func skillDetailSheet(_ skill: SkillInfo) -> some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(spacing: 8) {
                            Text(skill.name)
                                .font(.system(size: 20, weight: .bold))
                            
                            Text(skill.locationCategory)
                                .font(.system(size: 11, weight: .medium))
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(skill.isBuiltin ? Color.gray.opacity(0.12) : Color.indigo.opacity(0.12))
                                .foregroundColor(skill.isBuiltin ? .secondary : .indigo)
                                .clipShape(Capsule())
                        }
                        
                        if let path = skill.path {
                            Text(path)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(.secondary)
                        }
                    }
                    
                    if let desc = skill.description, !desc.isEmpty {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("概述")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundColor(.secondary)
                            Text(desc)
                                .font(.system(size: 14))
                                .foregroundColor(.primary)
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color(.secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 10))
                    }
                    
                    if let content = skill.content, !content.isEmpty {
                        VStack(alignment: .leading, spacing: 6) {
                            Text("SKILL.md 规则指令")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundColor(.secondary)
                            
                            Text(content)
                                .font(.system(size: 12.5, design: .monospaced))
                                .foregroundColor(.primary)
                                .padding(12)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .background(Color(.tertiarySystemBackground))
                                .clipShape(RoundedRectangle(cornerRadius: 10))
                        }
                    }
                }
                .padding(16)
            }
            .navigationTitle("技能详情")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDragIndicator(.visible)
    }
    
    // MARK: - Plugins Section

    private var filteredPlugins: [CascadePluginItem] {
        let q = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty { return plugins }
        return plugins.filter { p in
            p.id.lowercased().contains(q) ||
            p.name.lowercased().contains(q) ||
            p.description.lowercased().contains(q)
        }
    }

    @ViewBuilder
    private var pluginsSection: some View {
        if !pluginProviders.isEmpty {
            Section(header: Text("已授权服务账号")) {
                ForEach(pluginProviders) { provider in
                    HStack {
                        Image(systemName: provider.signedIn ? "checkmark.seal.fill" : "exclamationmark.circle")
                            .foregroundColor(provider.signedIn ? .green : .orange)
                        Text(provider.name)
                            .font(.system(size: 14, weight: .medium))
                        Spacer()
                        Text(provider.signedIn ? String(localized: "已登录") : String(localized: "需重连"))
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(provider.signedIn ? .green : .orange)
                    }
                }
            }
        }

        if !pluginPermissionGrants.isEmpty {
            Section(header: Text("已授权插件权限 (\(pluginPermissionGrants.count))")) {
                ForEach(pluginPermissionGrants) { item in
                    HStack(alignment: .center, spacing: 10) {
                        VStack(alignment: .leading, spacing: 3) {
                            HStack(spacing: 6) {
                                Text(item.displayTitle)
                                    .font(.system(size: 13.5, weight: .semibold))
                                    .lineLimit(2)
                                Text(item.scopeLabel)
                                    .font(.system(size: 10.5, weight: .medium))
                                    .foregroundColor(.indigo)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background(Color.indigo.opacity(0.12))
                                    .clipShape(Capsule())
                            }
                            if let res = item.resourceName, !res.isEmpty {
                                Text(res)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundColor(.secondary)
                                    .lineLimit(1)
                            } else if item.anyResource {
                                Text("全部资源")
                                    .font(.system(size: 11))
                                    .foregroundColor(.secondary)
                            }
                        }
                        Spacer(minLength: 8)
                        Button(role: .destructive) {
                            revokePluginGrant(item)
                        } label: {
                            if revokingGrantIds.contains(item.id) {
                                ProgressView().scaleEffect(0.7)
                            } else {
                                Text("撤销")
                                    .font(.system(size: 12, weight: .semibold))
                            }
                        }
                        .buttonStyle(.bordered)
                        .disabled(revokingGrantIds.contains(item.id))
                    }
                    .padding(.vertical, 2)
                }
            }
        }

        if filteredPlugins.isEmpty {
            Section {
                VStack(spacing: 8) {
                    Image(systemName: "puzzlepiece.extension")
                        .font(.system(size: 32))
                        .foregroundColor(.secondary)
                    Text("暂无插件或未找到匹配项")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
            }
        } else {
            Section(header: Text("已装载插件 (\(filteredPlugins.count))")) {
                ForEach(filteredPlugins) { plugin in
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(alignment: .center, spacing: 8) {
                            Text(plugin.name)
                                .font(.system(size: 15, weight: .bold))
                                .lineLimit(1)
                                .minimumScaleFactor(0.9)

                            Spacer(minLength: 8)

                            if !plugin.installed {
                                Button(action: { installPlugin(plugin.id) }) {
                                    if installingPluginIds.contains(plugin.id) {
                                        ProgressView().scaleEffect(0.7)
                                    } else {
                                        Text("安装")
                                            .font(.system(size: 12, weight: .semibold))
                                            .lineLimit(1)
                                    }
                                }
                                .buttonStyle(.borderedProminent)
                                .fixedSize(horizontal: true, vertical: false)
                                .layoutPriority(1)
                                .disabled(installingPluginIds.contains(plugin.id))
                            } else if (plugin.id.lowercased() == "swarm" || plugin.name.lowercased() == "swarm"),
                                      hasActiveSwarm,
                                      let onOpenSwarm {
                                Button(action: {
                                    dismiss()
                                    onOpenSwarm()
                                }) {
                                    Text("打开看板")
                                        .font(.system(size: 12, weight: .semibold))
                                        .lineLimit(1)
                                }
                                .buttonStyle(.borderedProminent)
                                .fixedSize(horizontal: true, vertical: false)
                                .layoutPriority(1)
                            }
                        }

                        HStack(spacing: 6) {
                            if plugin.installed {
                                Text(plugin.enabled ? String(localized: "已启用") : String(localized: "已停用"))
                                    .font(.system(size: 10.5, weight: .medium))
                                    .lineLimit(1)
                                    .fixedSize(horizontal: true, vertical: false)
                                    .foregroundColor(plugin.enabled ? .green : .secondary)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 1.5)
                                    .background((plugin.enabled ? Color.green : Color.secondary).opacity(0.12))
                                    .clipShape(Capsule())
                            }

                            if let ver = plugin.version, !ver.isEmpty {
                                Text("v\(ver)")
                                    .font(.system(size: 10.5, design: .monospaced))
                                    .foregroundColor(.secondary)
                                    .fixedSize(horizontal: true, vertical: false)
                            }

                            Text(plugin.id)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }

                        if !plugin.description.isEmpty {
                            Text(plugin.description)
                                .font(.system(size: 12.5))
                                .foregroundColor(.secondary)
                                .lineLimit(3)
                        }
                    }
                    .padding(.vertical, 4)
                }
            }
        }
    }

    private func installPlugin(_ id: String) {
        guard let url = settings.serverURL else { return }
        installingPluginIds.insert(id)
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            _ = try? await APIClient.shared.installPlugin(id: id, conversationId: currentConversationId, autoResume: true, baseURL: url)
            if let refreshed = try? await APIClient.shared.fetchPlugins(baseURL: url) {
                self.plugins = refreshed.plugins
                self.pluginProviders = refreshed.providers
                self.pluginPermissionGrants = refreshed.permissionGrants
            }
            installingPluginIds.remove(id)
        }
    }

    private func revokePluginGrant(_ item: PluginPermissionGrantItem) {
        guard let url = settings.serverURL else { return }
        revokingGrantIds.insert(item.id)
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            do {
                try await APIClient.shared.revokePluginPermissionGrant(item, baseURL: url)
                self.pluginPermissionGrants.removeAll { $0.id == item.id }
            } catch {
                self.errorMessage = error.localizedDescription
            }
            revokingGrantIds.remove(item.id)
        }
    }

    // MARK: - Automations Section

    private var filteredAutomations: [AutomationSidecarItem] {
        let q = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty { return automations }
        return automations.filter { a in
            a.name.lowercased().contains(q) ||
            (a.description?.lowercased().contains(q) == true) ||
            (a.cronExpression?.lowercased().contains(q) == true) ||
            (a.prompt?.lowercased().contains(q) == true)
        }
    }

    @ViewBuilder
    private var automationsSection: some View {
        Section {
            Button {
                newAutoName = ""
                newAutoSchedule = "0 9 * * *"
                newAutoPrompt = ""
                showCreateAutomation = true
            } label: {
                Label("新建定时任务", systemImage: "plus.circle.fill")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.indigo)
            }
        }

        if filteredAutomations.isEmpty && automationEvents.isEmpty {
            Section {
                VStack(spacing: 8) {
                    Image(systemName: "clock.arrow.2.circlepath")
                        .font(.system(size: 32))
                        .foregroundColor(.secondary)
                    Text("暂无定时自动化任务或未找到匹配项")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
            }
        } else {
            if !filteredAutomations.isEmpty {
                Section(header: Text("定时计划 (\(filteredAutomations.count))")) {
                    ForEach(filteredAutomations) { sidecar in
                        VStack(alignment: .leading, spacing: 6) {
                            HStack(alignment: .center, spacing: 8) {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(sidecar.name)
                                        .font(.system(size: 15, weight: .bold))
                                        .lineLimit(2)
                                        .fixedSize(horizontal: false, vertical: true)

                                    HStack(spacing: 6) {
                                        Text(sidecar.isRunning ? String(localized: "运行中") : String(localized: "已暂停"))
                                            .font(.system(size: 10.5, weight: .medium))
                                            .lineLimit(1)
                                            .fixedSize(horizontal: true, vertical: false)
                                            .layoutPriority(1)
                                            .foregroundColor(sidecar.isRunning ? .green : .secondary)
                                            .padding(.horizontal, 6)
                                            .padding(.vertical, 1.5)
                                            .background((sidecar.isRunning ? Color.green : Color.secondary).opacity(0.12))
                                            .clipShape(Capsule())

                                        if let cron = sidecar.cronExpression, !cron.isEmpty {
                                            HStack(spacing: 3) {
                                                Image(systemName: "clock")
                                                    .font(.system(size: 10))
                                                Text(AutomationCronHelper.humanSummary(cron))
                                                    .font(.system(size: 11.5))
                                                    .lineLimit(1)
                                            }
                                            .foregroundColor(.secondary)
                                        }
                                    }
                                }
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .contentShape(Rectangle())
                                .onTapGesture {
                                    selectedAutomationId = sidecar.id
                                }

                                if sidecar.isRunning {
                                    Button(action: {
                                        runNowAutomation(sidecar)
                                    }) {
                                        Image(systemName: "play.fill")
                                            .font(.system(size: 11, weight: .semibold))
                                            .frame(width: 28, height: 28)
                                            .background(Color.indigo.opacity(0.12))
                                            .foregroundColor(.indigo)
                                            .clipShape(Circle())
                                    }
                                    .buttonStyle(.borderless)
                                    .accessibilityLabel(String(localized: "立即运行 \(sidecar.name)"))
                                    .disabled(managingAutomationNames.contains(sidecar.id))
                                }

                                Button(action: {
                                    toggleAutomation(sidecar)
                                }) {
                                    Group {
                                        if managingAutomationNames.contains(sidecar.id) {
                                            ProgressView().scaleEffect(0.7)
                                        } else {
                                            Text(sidecar.isRunning ? String(localized: "停止") : String(localized: "启动"))
                                                .font(.system(size: 12, weight: .semibold))
                                                .lineLimit(1)
                                        }
                                    }
                                    .padding(.horizontal, 10)
                                    .padding(.vertical, 5)
                                    .background(sidecar.isRunning ? Color.secondary.opacity(0.14) : Color.indigo.opacity(0.14))
                                    .foregroundColor(sidecar.isRunning ? .primary : .indigo)
                                    .clipShape(Capsule())
                                    .fixedSize(horizontal: true, vertical: false)
                                }
                                .buttonStyle(.borderless)
                                .disabled(managingAutomationNames.contains(sidecar.id))

                                Image(systemName: "chevron.right")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.secondary.opacity(0.5))
                                    .onTapGesture {
                                        selectedAutomationId = sidecar.id
                                    }
                            }

                            VStack(alignment: .leading, spacing: 4) {
                                if sidecar.nextRunAt?.isEmpty == false || sidecar.lastRunAt?.isEmpty == false {
                                    HStack(spacing: 10) {
                                        if sidecar.isRunning, let next = sidecar.nextRunAt, !next.isEmpty {
                                            Text("下次: \(AutomationCronHelper.formatTimestamp(next))")
                                                .font(.system(size: 11))
                                                .foregroundColor(.secondary)
                                                .lineLimit(1)
                                                .minimumScaleFactor(0.85)
                                        }
                                        if let last = sidecar.lastRunAt, !last.isEmpty {
                                            Text("上次: \(AutomationCronHelper.formatTimestamp(last))")
                                                .font(.system(size: 11))
                                                .foregroundColor(.secondary.opacity(0.8))
                                                .lineLimit(1)
                                                .minimumScaleFactor(0.85)
                                        }
                                    }
                                }

                                if let desc = sidecar.description ?? sidecar.prompt, !desc.isEmpty {
                                    Text(desc)
                                        .font(.system(size: 12.5))
                                        .foregroundColor(.secondary)
                                        .lineLimit(2)
                                }
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                selectedAutomationId = sidecar.id
                            }
                        }
                        .padding(.vertical, 4)
                        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                            Button(role: .destructive) {
                                pendingDeleteAutomation = sidecar
                            } label: {
                                Label("删除", systemImage: "trash")
                            }
                        }
                    }
                }
            }

            if !automationEvents.isEmpty {
                Section(header: Text("最近触发记录")) {
                    ForEach(automationEvents) { ev in
                        Button(action: {
                            if let cid = ev.conversationId, !cid.isEmpty {
                                dismiss()
                                onSelectConversation?(cid)
                            }
                        }) {
                            VStack(alignment: .leading, spacing: 4) {
                                HStack {
                                    Text(ev.scheduleId)
                                        .font(.system(size: 13.5, weight: .semibold))
                                        .foregroundColor(.primary)
                                    Spacer()
                                    if !ev.timestamp.isEmpty {
                                        Text(AutomationCronHelper.formatTimestamp(ev.timestamp))
                                            .font(.system(size: 11))
                                            .foregroundColor(.secondary)
                                    }
                                }
                                if let msg = ev.message, !msg.isEmpty {
                                    Text(msg)
                                        .font(.system(size: 12))
                                        .foregroundColor(.secondary)
                                        .lineLimit(2)
                                }
                            }
                            .padding(.vertical, 2)
                        }
                        .disabled(ev.conversationId?.isEmpty != false)
                    }
                }
            }
        }
    }

    private func toggleAutomation(_ sidecar: AutomationSidecarItem) {
        guard let url = settings.serverURL else { return }
        let action = sidecar.isRunning ? "stop" : "start"
        managingAutomationNames.insert(sidecar.id)
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            defer { managingAutomationNames.remove(sidecar.id) }
            do {
                _ = try await APIClient.shared.manageAutomation(action: action, sidecarId: sidecar.id, baseURL: url)
                if let refreshed = try? await APIClient.shared.fetchAutomations(baseURL: url) {
                    self.automations = refreshed.automations
                    self.automationEvents = refreshed.events
                }
            } catch {
                actionErrorMessage = error.localizedDescription
            }
        }
    }

    private func runNowAutomation(_ sidecar: AutomationSidecarItem) {
        guard let url = settings.serverURL else { return }
        managingAutomationNames.insert(sidecar.id)
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            defer { managingAutomationNames.remove(sidecar.id) }
            do {
                _ = try await APIClient.shared.manageAutomation(action: "run_now", sidecarId: sidecar.id, baseURL: url)
                if let refreshed = try? await APIClient.shared.fetchAutomations(baseURL: url) {
                    self.automations = refreshed.automations
                    self.automationEvents = refreshed.events
                }
            } catch {
                actionErrorMessage = error.localizedDescription
            }
        }
    }

    // MARK: - Background Tasks Section

    private var filteredBackgroundTasks: [GlobalBackgroundTaskItem] {
        let q = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty { return backgroundTasks }
        return backgroundTasks.filter { t in
            t.displayTitle.lowercased().contains(q) ||
            t.commandLine.lowercased().contains(q) ||
            t.conversationTitle.lowercased().contains(q) ||
            (t.workspaceName?.lowercased().contains(q) == true)
        }
    }

    @ViewBuilder
    private var backgroundTasksSection: some View {
        let list = filteredBackgroundTasks
        if isLoading && backgroundTasks.isEmpty {
            Section {
                VStack(spacing: 10) {
                    ProgressView()
                    Text("正在同步后台任务...")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 28)
            }
        } else if list.isEmpty {
            Section {
                VStack(spacing: 10) {
                    Image(systemName: "terminal")
                        .font(.system(size: 32))
                        .foregroundColor(.secondary)
                    Text(searchText.isEmpty ? String(localized: "当前没有正在运行的后台任务") : String(localized: "未找到匹配的后台任务"))
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(.secondary)
                    if searchText.isEmpty {
                        Text("各会话中执行的后台终端命令、常驻开发服务器与定时任务会实时汇总在此处")
                            .font(.system(size: 12))
                            .foregroundColor(Color(.tertiaryLabel))
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 24)
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 28)
            }
        } else {
            if list.count > 1 {
                Section {
                    Button(role: .destructive) {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        showStopAllTasksConfirm = true
                    } label: {
                        HStack {
                            Image(systemName: "stop.circle.fill")
                            Text("一键停止全部后台任务 (\(list.count))")
                                .font(.system(size: 14, weight: .semibold))
                            Spacer()
                        }
                    }
                }
            }

            let currentTasks = list.filter { $0.conversationId == currentConversationId }
            let otherTasks = list.filter { $0.conversationId != currentConversationId }

            if !currentTasks.isEmpty {
                Section(header: Text("当前会话任务 (\(currentTasks.count))")) {
                    ForEach(currentTasks) { task in
                        backgroundTaskRow(task, isCurrentSession: true)
                    }
                }
            }

            if !otherTasks.isEmpty {
                Section(header: Text(currentConversationId != nil ? String(localized: "其他会话任务 (\(otherTasks.count))") : String(localized: "正在运行的后台任务 (\(otherTasks.count))"))) {
                    ForEach(otherTasks) { task in
                        backgroundTaskRow(task, isCurrentSession: false)
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func backgroundTaskRow(_ task: GlobalBackgroundTaskItem, isCurrentSession: Bool) -> some View {
        let isStopping = stoppingTaskIds.contains(task.id)
        let isLogExpanded = expandedLogTaskIds.contains(task.id)
        let isLoadingLog = loadingLogTaskIds.contains(task.id)
        let isCopied = copiedTaskId == task.id

        VStack(alignment: .leading, spacing: 8) {
            // 1. Header: Type Badge + Title + Elapsed + Stop Button
            HStack(alignment: .center, spacing: 6) {
                let badgeTitle = task.isDaemon ? String(localized: "常驻守护") : (task.toolName == "schedule" ? String(localized: "定时任务") : String(localized: "后台命令"))
                let badgeColor: Color = task.isDaemon ? .purple : (task.toolName == "schedule" ? .orange : .indigo)

                Text(badgeTitle)
                    .font(.system(size: 10.5, weight: .semibold))
                    .lineLimit(1)
                    .fixedSize(horizontal: true, vertical: false)
                    .foregroundColor(badgeColor)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(badgeColor.opacity(0.12))
                    .clipShape(Capsule())

                Text(task.displayTitle)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)

                if let elapsed = task.elapsedText {
                    Text(elapsed)
                        .font(.system(size: 11, weight: .medium, design: .monospaced))
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color(.tertiarySystemFill))
                        .clipShape(Capsule())
                }

                Spacer()

                Button {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    if task.isDaemon {
                        pendingStopDaemonTask = task
                    } else {
                        Task { await stopSingleTask(task) }
                    }
                } label: {
                    if isStopping {
                        ProgressView().controlSize(.small)
                    } else {
                        Image(systemName: "stop.circle.fill")
                            .font(.system(size: 20))
                            .foregroundColor(.red.opacity(0.9))
                    }
                }
                .buttonStyle(.plain)
                .accessibilityLabel(String(localized: "停止任务 \(task.displayTitle)"))
                .disabled(isStopping)
            }

            // 2. Session & Workspace Bar (Clickable to jump when not current session)
            Button {
                guard !isCurrentSession, let onSelect = onSelectConversation else { return }
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                dismiss()
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                    onSelect(task.conversationId)
                }
            } label: {
                HStack(spacing: 5) {
                    Image(systemName: isCurrentSession ? "bubble.left.fill" : "folder.fill")
                        .font(.system(size: 11))
                        .foregroundColor(.indigo)
                    if let ws = task.workspaceName, !ws.isEmpty {
                        Text(ws)
                            .font(.system(size: 11.5, weight: .semibold))
                            .foregroundColor(.indigo)
                        Text("·")
                            .font(.system(size: 11.5))
                            .foregroundColor(.secondary)
                    }
                    Text(task.conversationTitle)
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                    Spacer()
                    if !isCurrentSession && onSelectConversation != nil {
                        HStack(spacing: 2) {
                            Text("跳转会话")
                                .font(.system(size: 11, weight: .medium))
                                .lineLimit(1)
                            Image(systemName: "chevron.right")
                                .font(.system(size: 9.5, weight: .bold))
                        }
                        .fixedSize(horizontal: true, vertical: false)
                        .layoutPriority(1)
                        .foregroundColor(.indigo)
                    }
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 5)
                .background(Color(.secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
            }
            .buttonStyle(.plain)

            // 3. Command Line + Copy + Log Toggle
            if !task.commandLine.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text(task.commandLine)
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.primary.opacity(0.88))
                        .lineLimit(3)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(8)
                        .background(Color(.tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))

                    HStack(spacing: 12) {
                        Button {
                            UIPasteboard.general.string = task.commandLine
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            copiedTaskId = task.id
                            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
                                if copiedTaskId == task.id { copiedTaskId = nil }
                            }
                        } label: {
                            HStack(spacing: 3) {
                                Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                                    .font(.system(size: 11))
                                Text(isCopied ? String(localized: "已复制命令") : String(localized: "复制命令"))
                                    .font(.system(size: 11.5, weight: .medium))
                                    .lineLimit(1)
                                    .minimumScaleFactor(0.85)
                            }
                            .foregroundColor(isCopied ? .green : .secondary)
                        }
                        .buttonStyle(.plain)

                        if let logUri = task.logUri, !logUri.isEmpty {
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                if isLogExpanded {
                                    expandedLogTaskIds.remove(task.id)
                                } else {
                                    expandedLogTaskIds.insert(task.id)
                                    Task { await fetchTaskLog(task) }
                                }
                            } label: {
                                HStack(spacing: 3) {
                                    Image(systemName: "text.alignleft")
                                        .font(.system(size: 11))
                                    Text(isLogExpanded ? String(localized: "收起终端日志") : String(localized: "查看最新终端日志"))
                                        .font(.system(size: 11.5, weight: .medium))
                                        .lineLimit(1)
                                        .minimumScaleFactor(0.85)
                                }
                                .foregroundColor(.indigo)
                            }
                            .buttonStyle(.plain)
                        }

                        if isLogExpanded {
                            Spacer(minLength: 4)
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                Task { await fetchTaskLog(task) }
                            } label: {
                                HStack(spacing: 3) {
                                    Image(systemName: "arrow.clockwise")
                                        .font(.system(size: 10.5, weight: .semibold))
                                    Text("刷新")
                                        .font(.system(size: 11, weight: .medium))
                                        .lineLimit(1)
                                }
                                .fixedSize(horizontal: true, vertical: false)
                                .foregroundColor(.secondary)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }

            // 4. Expanded Log Output Tail
            if isLogExpanded {
                VStack(alignment: .leading, spacing: 4) {
                    if isLoadingLog && taskLogs[task.id] == nil {
                        HStack(spacing: 6) {
                            ProgressView().controlSize(.small)
                            Text("正在读取日志尾部...")
                                .font(.system(size: 11.5))
                                .foregroundColor(.secondary)
                        }
                        .padding(8)
                    } else {
                        let logText = (taskLogs[task.id] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                        ScrollView(.vertical, showsIndicators: true) {
                            Text(logText.isEmpty ? String(localized: "（暂无终端日志输出）") : logText)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(.primary.opacity(0.85))
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .textSelection(.enabled)
                        }
                        .frame(maxHeight: 160)
                        .padding(8)
                        .background(Color(.secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    }
                }
            }
        }
        .padding(.vertical, 4)
    }

    private func fetchTaskLog(_ task: GlobalBackgroundTaskItem) async {
        guard let url = settings.serverURL else { return }
        loadingLogTaskIds.insert(task.id)
        defer { loadingLogTaskIds.remove(task.id) }
        if let resp = try? await APIClient.shared.fetchBackgroundTaskLog(
            conversationId: task.conversationId,
            taskId: task.taskId,
            logUri: task.logUri,
            baseURL: url
        ) {
            taskLogs[task.id] = resp.content
        }
    }

    private func stopSingleTask(_ task: GlobalBackgroundTaskItem) async {
        guard let url = settings.serverURL else { return }
        stoppingTaskIds.insert(task.id)
        defer { stoppingTaskIds.remove(task.id) }
        do {
            try await APIClient.shared.stopTask(
                cascadeId: task.conversationId,
                stepIndex: task.stepIndex,
                taskId: task.taskId,
                baseURL: url
            )
            backgroundTasks.removeAll { $0.id == task.id }
            onTasksChanged?(backgroundTasks)
            if let refreshed = try? await APIClient.shared.fetchGlobalBackgroundTasks(baseURL: url) {
                backgroundTasks = refreshed
                onTasksChanged?(refreshed)
            }
        } catch {
            actionErrorMessage = error.localizedDescription
        }
    }

    private func stopAllBackgroundTasks(_ tasks: [GlobalBackgroundTaskItem]) async {
        guard let url = settings.serverURL else { return }
        for t in tasks {
            stoppingTaskIds.insert(t.id)
        }
        var firstError: Error? = nil
        for t in tasks {
            do {
                try await APIClient.shared.stopTask(
                    cascadeId: t.conversationId,
                    stepIndex: t.stepIndex,
                    taskId: t.taskId,
                    baseURL: url
                )
            } catch {
                if firstError == nil { firstError = error }
            }
        }
        stoppingTaskIds.removeAll()
        if let refreshed = try? await APIClient.shared.fetchGlobalBackgroundTasks(baseURL: url) {
            backgroundTasks = refreshed
            onTasksChanged?(refreshed)
        } else if firstError == nil {
            backgroundTasks.removeAll()
            onTasksChanged?([])
        }
        if let firstError {
            actionErrorMessage = firstError.localizedDescription
        }
    }

    // MARK: - Error View
    
    private func errorStateView(_ msg: String) -> some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 40))
                .foregroundColor(.orange)
            Text("加载工具与技能失败")
                .font(.system(size: 16, weight: .semibold))
            Text(msg)
                .font(.system(size: 13))
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 32)
            
            Button("重试") {
                Task {
                    await loadData()
                }
            }
            .buttonStyle(.borderedProminent)
            .padding(.top, 8)
            Spacer()
        }
    }
    
    // MARK: - Data Loading
    
    private func loadData() async {
        isLoading = true
        errorMessage = nil
        
        // Offline Fallback when serverURL is missing
        if settings.serverURL == nil {
            applyDemoData()
            isLoading = false
            return
        }
        
        guard let url = settings.serverURL else {
            if mcpServers.isEmpty && skills.isEmpty {
                errorMessage = String(localized: "未连接网关")
            }
            isLoading = false
            return
        }
        
        async let fetchTasks = try? APIClient.shared.fetchGlobalBackgroundTasks(baseURL: url)
        async let fetchMcp = try? APIClient.shared.fetchMcpServerStates(baseURL: url)
        async let fetchSkills = try? APIClient.shared.fetchAllSkills(baseURL: url)
        async let fetchPlugins = try? APIClient.shared.fetchPlugins(baseURL: url)
        async let fetchAutomations = try? APIClient.shared.fetchAutomations(baseURL: url)
        async let fetchMemories = try? APIClient.shared.fetchMemories(baseURL: url)
        
        let (tasksResult, mcpResult, skillsResult, pluginsResult, automationsResult, memoriesResult) = await (fetchTasks, fetchMcp, fetchSkills, fetchPlugins, fetchAutomations, fetchMemories)
        
        if let tasksResult {
            self.backgroundTasks = tasksResult
            self.onTasksChanged?(tasksResult)
        }
        if let mcpResult {
            self.mcpServers = mcpResult
            CacheManager.shared.saveMcpServers(mcpResult)
        }
        if let skillsResult {
            self.skills = skillsResult
            CacheManager.shared.saveSkills(skillsResult)
        }
        if let pluginsResult {
            self.plugins = pluginsResult.plugins
            self.pluginProviders = pluginsResult.providers
            self.pluginPermissionGrants = pluginsResult.permissionGrants
        }
        if let automationsResult {
            self.automations = automationsResult.automations
            self.automationEvents = automationsResult.events
        }
        if let memoriesResult {
            self.memories = memoriesResult
        }
        
        if mcpResult == nil && skillsResult == nil && mcpServers.isEmpty && skills.isEmpty {
            if settings.isDemoMode {
                applyDemoData()
            } else if backgroundTasks.isEmpty {
                errorMessage = String(localized: "无法获取 MCP 服务与技能清单，请检查网关连接")
            }
        }
        isLoading = false
    }
    
    private func applyDemoData() {
        self.mcpServers = [
            McpServerState(
                spec: McpServerSpec(serverName: "github", command: "npx -y @modelcontextprotocol/server-github"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(
                        name: "search_repositories",
                        description: DemoGateway.t(
                            "检索 GitHub 开源仓库、Issue 与 Pull Request 上下文",
                            "Search GitHub repositories, issues, and pull requests",
                            "GitHub リポジトリ・Issue・Pull Request のコンテキストを検索"
                        ),
                        serverName: "github",
                        jsonSchemaString: "{\"properties\":{\"query\":{\"type\":\"string\",\"description\":\"Search query\"}},\"required\":[\"query\"]}"
                    ),
                    McpToolDefinition(
                        name: "get_file_contents",
                        description: DemoGateway.t(
                            "按分支与路径读取仓库源码文件内容",
                            "Read file contents by branch and path from a repository",
                            "ブランチとパスを指定してリポジトリのソースファイルを読み取り"
                        ),
                        serverName: "github",
                        jsonSchemaString: "{\"properties\":{\"owner\":{\"type\":\"string\"},\"repo\":{\"type\":\"string\"},\"path\":{\"type\":\"string\"}},\"required\":[\"owner\",\"repo\",\"path\"]}"
                    )
                ],
                serverInfo: McpServerInfoDetail(name: "github-mcp", version: "2.4.0"),
                instructions: DemoGateway.t(
                    "GitHub 仓库检索与 PR 协作工具集",
                    "GitHub repository search and pull request integration",
                    "GitHub リポジトリ検索および PR 連携ツールセット"
                )
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "weather", command: "uv run weather-server"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(
                        name: "get_current_weather",
                        description: DemoGateway.t(
                            "获取指定城市的当前实时天气情况",
                            "Fetch current weather conditions for a city",
                            "指定した都市の現在の天気情報を取得"
                        ),
                        serverName: "weather",
                        jsonSchemaString: "{\"properties\":{\"city\":{\"type\":\"string\",\"description\":\"City name\"}},\"required\":[\"city\"]}"
                    )
                ],
                serverInfo: McpServerInfoDetail(name: "weather-service", version: "1.2.0"),
                instructions: DemoGateway.t(
                    "全球实时与未来天气查询",
                    "Global real-time and forecast weather service",
                    "世界のリアルタイム天気・予報検索サービス"
                )
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "postgres", command: "uvx mcp-postgres-readonly"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(
                        name: "explain_query",
                        description: DemoGateway.t(
                            "执行只读 EXPLAIN ANALYZE 查询计划诊断",
                            "Run read-only EXPLAIN ANALYZE query plan diagnostics",
                            "読み取り専用の EXPLAIN ANALYZE 実行計画診断を実行"
                        ),
                        serverName: "postgres",
                        jsonSchemaString: "{\"properties\":{\"sql\":{\"type\":\"string\"}},\"required\":[\"sql\"]}"
                    )
                ],
                serverInfo: McpServerInfoDetail(name: "postgres-mcp", version: "1.8.0"),
                instructions: DemoGateway.t(
                    "PostgreSQL 只读 Schema 与执行计划排查",
                    "Read-only PostgreSQL schema and query-plan inspector",
                    "PostgreSQL 読み取り専用スキーマ＆実行計画インスペクター"
                )
            )
        ]
        self.skills = [
            SkillInfo(
                name: "chart-generator",
                description: DemoGateway.t(
                    "自动化图表生成与多维度可视化分析",
                    "Automated data visualization and chart rendering",
                    "自動グラフ生成と多次元データ可視化"
                ),
                path: "builtin/skills/chart-generator/SKILL.md",
                content: DemoGateway.t(
                    "# Chart Generator\n自动根据输入数据渲染交互图表",
                    "# Chart Generator\nRender interactive charts directly from structured datasets.",
                    "# Chart Generator\n構造化データからインタラクティブなグラフを自動描画します。"
                )
            ),
            SkillInfo(
                name: "sql-inspector",
                description: DemoGateway.t(
                    "SQL 查询调优与执行计划诊断",
                    "SQL query tuning and execution plan diagnostics",
                    "SQL クエリチューニングと実行計画診断"
                ),
                path: "builtin/skills/sql-inspector/SKILL.md",
                content: DemoGateway.t(
                    "# SQL Inspector\n分析 SQL 慢查询并提出索引优化方案",
                    "# SQL Inspector\nAnalyze slow SQL queries and recommend safe indexing strategies.",
                    "# SQL Inspector\nスロークエリを解析し、安全なインデックス最適化案を提案します。"
                )
            ),
            SkillInfo(
                name: "swift-concurrency-audit",
                description: DemoGateway.t(
                    "Swift 6 并发模型与 Actor 隔离静态审查",
                    "Swift 6 concurrency and actor-isolation audit workflow",
                    "Swift 6 並行処理モデルと Actor 分離の静的監査"
                ),
                path: "config/skills/swift-concurrency-audit/SKILL.md",
                content: DemoGateway.t(
                    "# Swift Concurrency Audit\n检查 @MainActor 隔离、Sendable 合规性与取消传递",
                    "# Swift Concurrency Audit\nVerify @MainActor isolation, Sendable conformance, and task cancellation.",
                    "# Swift Concurrency Audit\n@MainActor 分離・Sendable 準拠・キャンセル伝播を検証します。"
                )
            )
        ]
    }

    // MARK: - Phase 3 Helpers & Sheets

    private func toggleMcp(_ name: String, enabled: Bool) {
        guard let url = settings.gatewayURL else { return }
        togglingMcpNames.insert(name)
        Task {
            defer { togglingMcpNames.remove(name) }
            do {
                try await APIClient.shared.toggleMcpServer(serverName: name, enabled: enabled, baseURL: url)
                if let refreshed = try? await APIClient.shared.fetchMcpServerStates(baseURL: url) {
                    self.mcpServers = refreshed
                    CacheManager.shared.saveMcpServers(refreshed)
                }
            } catch {
                actionErrorMessage = error.localizedDescription
            }
        }
    }

    private func deleteMemoryItem(_ item: MemoryRuleItem) {
        guard let url = settings.gatewayURL else { return }
        Task {
            do {
                let updated = try await APIClient.shared.deleteMemory(path: item.path, baseURL: url)
                memories = updated
            } catch {
                actionErrorMessage = error.localizedDescription
            }
        }
    }

    private func saveMemoryEditor() {
        guard let url = settings.gatewayURL else { return }
        let title = memTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        let content = memContent.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !title.isEmpty, !content.isEmpty else { return }
        isSavingMemory = true
        Task {
            defer { isSavingMemory = false }
            do {
                let updated = try await APIClient.shared.upsertMemory(
                    scope: memScope,
                    title: title,
                    content: content,
                    path: editingMemory?.path,
                    baseURL: url
                )
                memories = updated
                showMemoryEditor = false
            } catch {
                actionErrorMessage = error.localizedDescription
            }
        }
    }

    private func refreshAutomations() async {
        guard let url = settings.serverURL else { return }
        if let refreshed = try? await APIClient.shared.fetchAutomations(baseURL: url) {
            self.automations = refreshed.automations
            self.automationEvents = refreshed.events
        }
    }

    private func deleteAutomationItem(_ item: AutomationSidecarItem) {
        guard let url = settings.gatewayURL else { return }
        managingAutomationNames.insert(item.id)
        Task {
            defer { managingAutomationNames.remove(item.id) }
            do {
                try await APIClient.shared.deleteAutomation(sidecarId: item.id, baseURL: url)
                if let refreshed = try? await APIClient.shared.fetchAutomations(baseURL: url) {
                    automations = refreshed.automations
                    automationEvents = refreshed.events
                }
            } catch {
                actionErrorMessage = error.localizedDescription
            }
        }
    }

    private func submitCreateAutomation() {
        guard let url = settings.gatewayURL else { return }
        let name = newAutoName.trimmingCharacters(in: .whitespacesAndNewlines)
        let schedule = newAutoSchedule.trimmingCharacters(in: .whitespacesAndNewlines)
        let prompt = newAutoPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, !schedule.isEmpty, !prompt.isEmpty else { return }
        isCreatingAutomation = true
        Task {
            defer { isCreatingAutomation = false }
            do {
                try await APIClient.shared.createAutomation(name: name, schedule: schedule, prompt: prompt, baseURL: url)
                if let refreshed = try? await APIClient.shared.fetchAutomations(baseURL: url) {
                    automations = refreshed.automations
                    automationEvents = refreshed.events
                }
                showCreateAutomation = false
            } catch {
                actionErrorMessage = error.localizedDescription
            }
        }
    }

    private var memoryEditorSheet: some View {
        NavigationStack {
            Form {
                if editingMemory == nil {
                    Picker("作用域", selection: $memScope) {
                        Text("Agent 记忆").tag("memory")
                        Text("全局规则").tag("global")
                    }
                }
                Section("标题") {
                    TextField("规则或记忆标题", text: $memTitle)
                }
                Section("内容") {
                    TextEditor(text: $memContent)
                        .frame(minHeight: 160)
                }
            }
            .navigationTitle(editingMemory == nil ? String(localized: "新建记忆条目") : String(localized: "编辑记忆条目"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { showMemoryEditor = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isSavingMemory ? String(localized: "保存中...") : String(localized: "保存")) {
                        saveMemoryEditor()
                    }
                    .disabled(isSavingMemory || memTitle.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || memContent.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private var createAutomationSheet: some View {
        NavigationStack {
            Form {
                Section("任务名称") {
                    TextField("例如：daily-digest", text: $newAutoName)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                }
                Section("Cron 调度表达式") {
                    TextField("0 9 * * *", text: $newAutoSchedule)
                        .font(.system(size: 14, design: .monospaced))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                }
                Section("触发指令") {
                    TextEditor(text: $newAutoPrompt)
                        .frame(minHeight: 140)
                }
            }
            .navigationTitle("新建定时任务")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { showCreateAutomation = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isCreatingAutomation ? String(localized: "创建中...") : String(localized: "创建")) {
                        submitCreateAutomation()
                    }
                    .disabled(isCreatingAutomation || newAutoName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || newAutoSchedule.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || newAutoPrompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
        }
        .presentationDetents([.medium, .large])
    }
}

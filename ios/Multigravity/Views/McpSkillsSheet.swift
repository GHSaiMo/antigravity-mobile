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
            .sheet(item: $viewingSkill) { skill in
                skillDetailSheet(skill)
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
        case .mcp: return "搜索 MCP 服务或工具名称"
        case .skills: return "搜索技能名称或描述"
        case .plugins: return "搜索插件名称或描述"
        case .automations: return "搜索定时任务或表达式"
        case .tasks: return "搜索命令、会话标题或工作区"
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
                            
                            if let ver = server.serverInfo?.version, !ver.isEmpty {
                                Text("v\(ver)")
                                    .font(.system(size: 11, design: .monospaced))
                                    .padding(.horizontal, 5)
                                    .padding(.vertical, 1.5)
                                    .background(Color(.secondarySystemBackground))
                                    .clipShape(RoundedRectangle(cornerRadius: 4))
                                    .foregroundColor(.secondary)
                            }
                            
                            Spacer()
                            
                            HStack(spacing: 4) {
                                Circle()
                                    .fill(server.isReady ? Color.green : Color.orange)
                                    .frame(width: 7, height: 7)
                                Text(server.statusTitle)
                                    .font(.system(size: 11.5, weight: .medium))
                                    .foregroundColor(server.isReady ? .green : .orange)
                            }
                            .padding(.horizontal, 7)
                            .padding(.vertical, 3)
                            .background(server.isReady ? Color.green.opacity(0.1) : Color.orange.opacity(0.1))
                            .clipShape(Capsule())
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
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("关闭") {
                        viewingSkill = nil
                    }
                    .font(.system(size: 15, weight: .medium))
                }
            }
        }
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
                        Text(provider.signedIn ? "已登录" : "需重连")
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(provider.signedIn ? .green : .orange)
                    }
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

                            if plugin.installed {
                                Text(plugin.enabled ? "已启用" : "已停用")
                                    .font(.system(size: 11, weight: .medium))
                                    .foregroundColor(plugin.enabled ? .green : .secondary)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background((plugin.enabled ? Color.green : Color.secondary).opacity(0.12))
                                    .clipShape(Capsule())
                            }

                            if let ver = plugin.version, !ver.isEmpty {
                                Text("v\(ver)")
                                    .font(.system(size: 10.5, design: .monospaced))
                                    .foregroundColor(.secondary)
                            }

                            Spacer()

                            if !plugin.installed {
                                Button(action: { installPlugin(plugin.id) }) {
                                    if installingPluginIds.contains(plugin.id) {
                                        ProgressView().scaleEffect(0.7)
                                    } else {
                                        Text("安装")
                                            .font(.system(size: 12, weight: .semibold))
                                    }
                                }
                                .buttonStyle(.borderedProminent)
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
                                }
                                .buttonStyle(.borderedProminent)
                            }
                        }

                        Text(plugin.id)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundColor(.secondary)

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
            }
            installingPluginIds.remove(id)
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
                                Text(sidecar.name)
                                    .font(.system(size: 15, weight: .bold))

                                Text(sidecar.isRunning ? "运行中" : "已停止")
                                    .font(.system(size: 11, weight: .medium))
                                    .foregroundColor(sidecar.isRunning ? .green : .secondary)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background((sidecar.isRunning ? Color.green : Color.secondary).opacity(0.12))
                                    .clipShape(Capsule())

                                Spacer()

                                Button(action: {
                                    runNowAutomation(sidecar)
                                }) {
                                    Text("立即运行")
                                        .font(.system(size: 12, weight: .semibold))
                                }
                                .buttonStyle(.bordered)
                                .disabled(managingAutomationNames.contains(sidecar.id))

                                Button(action: {
                                    toggleAutomation(sidecar)
                                }) {
                                    if managingAutomationNames.contains(sidecar.id) {
                                        ProgressView().scaleEffect(0.7)
                                    } else {
                                        Text(sidecar.isRunning ? "停止" : "启动")
                                            .font(.system(size: 12, weight: .semibold))
                                    }
                                }
                                .buttonStyle(.bordered)
                                .disabled(managingAutomationNames.contains(sidecar.id))
                            }

                            if let cron = sidecar.cronExpression, !cron.isEmpty {
                                HStack(spacing: 4) {
                                    Image(systemName: "clock")
                                        .font(.system(size: 10))
                                    Text(cron)
                                        .font(.system(size: 11, design: .monospaced))
                                }
                                .foregroundColor(.secondary)
                            }

                            if sidecar.nextRunAt?.isEmpty == false || sidecar.lastRunAt?.isEmpty == false {
                                HStack(spacing: 8) {
                                    if let next = sidecar.nextRunAt, !next.isEmpty {
                                        Text("下次: \(next)")
                                            .font(.system(size: 11))
                                            .foregroundColor(.secondary)
                                    }
                                    if let last = sidecar.lastRunAt, !last.isEmpty {
                                        Text("上次: \(last)")
                                            .font(.system(size: 11))
                                            .foregroundColor(.secondary.opacity(0.8))
                                    }
                                }
                            }

                            if let desc = sidecar.description ?? sidecar.prompt, !desc.isEmpty {
                                Text(desc)
                                    .font(.system(size: 12.5))
                                    .foregroundColor(.secondary)
                                    .lineLimit(3)
                            }
                        }
                        .padding(.vertical, 4)
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
                                        Text(ev.timestamp)
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
            _ = try? await APIClient.shared.manageAutomation(action: action, sidecarId: sidecar.id, baseURL: url)
            if let refreshed = try? await APIClient.shared.fetchAutomations(baseURL: url) {
                self.automations = refreshed.automations
                self.automationEvents = refreshed.events
            }
            managingAutomationNames.remove(sidecar.id)
        }
    }

    private func runNowAutomation(_ sidecar: AutomationSidecarItem) {
        guard let url = settings.serverURL else { return }
        managingAutomationNames.insert(sidecar.id)
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            _ = try? await APIClient.shared.manageAutomation(action: "run_now", sidecarId: sidecar.id, baseURL: url)
            if let refreshed = try? await APIClient.shared.fetchAutomations(baseURL: url) {
                self.automations = refreshed.automations
                self.automationEvents = refreshed.events
            }
            managingAutomationNames.remove(sidecar.id)
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
                    Text(searchText.isEmpty ? "当前没有正在运行的后台任务" : "未找到匹配的后台任务")
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
                        Task { await stopAllBackgroundTasks(list) }
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
                Section(header: Text(currentConversationId != nil ? "其他会话任务 (\(otherTasks.count))" : "正在运行的后台任务 (\(otherTasks.count))")) {
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
                let badgeTitle = task.isDaemon ? "常驻守护" : (task.toolName == "schedule" ? "定时任务" : "后台命令")
                let badgeColor: Color = task.isDaemon ? .purple : (task.toolName == "schedule" ? .orange : .indigo)

                Text(badgeTitle)
                    .font(.system(size: 10.5, weight: .semibold))
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
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color(.tertiarySystemFill))
                        .clipShape(Capsule())
                }

                Spacer()

                Button {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    Task { await stopSingleTask(task) }
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
                            Image(systemName: "chevron.right")
                                .font(.system(size: 9.5, weight: .bold))
                        }
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
                                Text(isCopied ? "已复制命令" : "复制命令")
                                    .font(.system(size: 11.5, weight: .medium))
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
                                    Text(isLogExpanded ? "收起终端日志" : "查看最新终端日志")
                                        .font(.system(size: 11.5, weight: .medium))
                                }
                                .foregroundColor(.indigo)
                            }
                            .buttonStyle(.plain)
                        }

                        if isLogExpanded {
                            Spacer()
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                Task { await fetchTaskLog(task) }
                            } label: {
                                HStack(spacing: 3) {
                                    Image(systemName: "arrow.clockwise")
                                        .font(.system(size: 10.5, weight: .semibold))
                                    Text("刷新")
                                        .font(.system(size: 11, weight: .medium))
                                }
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
                            Text(logText.isEmpty ? "（暂无终端日志输出）" : logText)
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
        try? await APIClient.shared.stopTask(
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
    }

    private func stopAllBackgroundTasks(_ tasks: [GlobalBackgroundTaskItem]) async {
        guard let url = settings.serverURL else { return }
        for t in tasks {
            stoppingTaskIds.insert(t.id)
        }
        for t in tasks {
            try? await APIClient.shared.stopTask(
                cascadeId: t.conversationId,
                stepIndex: t.stepIndex,
                taskId: t.taskId,
                baseURL: url
            )
        }
        stoppingTaskIds.removeAll()
        backgroundTasks.removeAll()
        onTasksChanged?([])
        if let refreshed = try? await APIClient.shared.fetchGlobalBackgroundTasks(baseURL: url) {
            backgroundTasks = refreshed
            onTasksChanged?(refreshed)
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
        
        // Demo Mode Fallback
        if settings.isDemoMode || settings.serverURL == nil {
            applyDemoData()
            isLoading = false
            return
        }
        
        guard let url = settings.serverURL else {
            if mcpServers.isEmpty && skills.isEmpty {
                errorMessage = "未连接网关"
            }
            isLoading = false
            return
        }
        
        async let fetchTasks = try? APIClient.shared.fetchGlobalBackgroundTasks(baseURL: url)
        async let fetchMcp = try? APIClient.shared.fetchMcpServerStates(baseURL: url)
        async let fetchSkills = try? APIClient.shared.fetchAllSkills(baseURL: url)
        async let fetchPlugins = try? APIClient.shared.fetchPlugins(baseURL: url)
        async let fetchAutomations = try? APIClient.shared.fetchAutomations(baseURL: url)
        
        let (tasksResult, mcpResult, skillsResult, pluginsResult, automationsResult) = await (fetchTasks, fetchMcp, fetchSkills, fetchPlugins, fetchAutomations)
        
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
        }
        if let automationsResult {
            self.automations = automationsResult.automations
            self.automationEvents = automationsResult.events
        }
        
        if mcpResult == nil && skillsResult == nil && mcpServers.isEmpty && skills.isEmpty {
            if settings.isDemoMode {
                applyDemoData()
            } else if backgroundTasks.isEmpty {
                errorMessage = "无法获取 MCP 服务与技能清单，请检查网关连接"
            }
        }
        isLoading = false
    }
    
    private func applyDemoData() {
        self.mcpServers = [
            McpServerState(
                spec: McpServerSpec(serverName: "ifind", command: "python3 -m ifind_mcp.server run"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "get_realtime_quotation", description: "查询证券实时行情、最新价、涨跌幅、盘口及资金流", serverName: "ifind", jsonSchemaString: "{\"properties\":{\"codes\":{\"type\":\"string\",\"description\":\"证券代码，如 600519.SH\"}},\"required\":[\"codes\"]}"),
                    McpToolDefinition(name: "get_history_quotation", description: "查询历史K线行情与序列数据", serverName: "ifind", jsonSchemaString: "{\"properties\":{\"codes\":{\"type\":\"string\"},\"period\":{\"type\":\"string\",\"default\":\"D\"}},\"required\":[\"codes\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "ifind-mcp", version: "4.0.0"),
                instructions: "iFinD 金融数据终端服务接口"
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "weather", command: "uv run weather-server"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "get_current_weather", description: "获取指定城市的当前实时天气情况", serverName: "weather", jsonSchemaString: "{\"properties\":{\"city\":{\"type\":\"string\",\"description\":\"城市名称\"}},\"required\":[\"city\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "weather-service", version: "1.2.0"),
                instructions: "全球实时与未来天气查询"
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "message_hub", command: "node message_hub.js"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "send_message", description: "统一向微信或元宝渠道发送通知与长图", serverName: "message_hub", jsonSchemaString: "{\"properties\":{\"target\":{\"type\":\"string\"},\"text\":{\"type\":\"string\"}},\"required\":[\"target\",\"text\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "message-hub", version: "2.1.0"),
                instructions: "统一消息与多端通信路由"
            )
        ]
        self.skills = [
            SkillInfo(name: "chart-generator", description: "自动化图表生成与多维度可视化分析", path: "builtin/skills/chart-generator/SKILL.md", content: "# Chart Generator\n自动根据输入数据渲染交互图表"),
            SkillInfo(name: "sql-inspector", description: "SQL 查询调优与执行计划诊断", path: "builtin/skills/sql-inspector/SKILL.md", content: "# SQL Inspector\n分析 SQL 慢查询并提出索引优化方案"),
            SkillInfo(name: "jiuge-market-analysis", description: "首席宏观研判与市场逻辑观点体系", path: "config/skills/jiuge-market-analysis/SKILL.md", content: "# Jiuge Market Analysis\n宏观与大宗商品核心驱动逻辑")
        ]
    }
}

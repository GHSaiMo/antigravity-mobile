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
    
    @State private var expandedServerNames: Set<String> = []
    @State private var expandedToolIds: Set<String> = []
    @State private var viewingSkill: SkillInfo? = nil
    
    public enum Tab: String, CaseIterable, Identifiable {
        case mcp = "MCP 服务"
        case skills = "扩展技能"
        public var id: String { rawValue }
    }
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Segmented control
                Picker("类别", selection: $selectedTab) {
                    Text("MCP 服务 (\(mcpServers.count))").tag(Tab.mcp)
                    Text("技能清单 (\(skills.count))").tag(Tab.skills)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.top, 10)
                .padding(.bottom, 8)
                
                // Summary bar
                summaryHeader
                    .padding(.horizontal, 16)
                    .padding(.bottom, 8)
                
                // Content
                if isLoading && mcpServers.isEmpty && skills.isEmpty {
                    Spacer()
                    ProgressView("正在检测已连接服务与技能...")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                    Spacer()
                } else if let error = errorMessage, mcpServers.isEmpty && skills.isEmpty {
                    errorStateView(error)
                } else {
                    List {
                        if selectedTab == .mcp {
                            mcpServersSection
                        } else {
                            skillsSection
                        }
                    }
                    .listStyle(.insetGrouped)
                    .searchable(text: $searchText, prompt: selectedTab == .mcp ? "搜索 MCP 服务或工具名称" : "搜索技能名称或描述")
                    .refreshable {
                        await loadData()
                    }
                }
            }
            .navigationTitle("工具与技能看板")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") {
                        dismiss()
                    }
                    .font(.system(size: 16, weight: .semibold))
                }
            }
            .sheet(item: $viewingSkill) { skill in
                skillDetailSheet(skill)
            }
            .task {
                await loadData()
            }
        }
    }
    
    // MARK: - Summary Header
    
    private var summaryHeader: some View {
        HStack(spacing: 12) {
            let readyCount = mcpServers.filter { $0.isReady }.count
            let totalTools = mcpServers.reduce(0) { $0 + $1.toolCount }
            
            HStack(spacing: 6) {
                Circle()
                    .fill(readyCount > 0 ? Color.green : Color.orange)
                    .frame(width: 8, height: 8)
                Text("\(readyCount)/\(mcpServers.count) 服务就绪")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.primary)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(Color(.secondarySystemBackground))
            .clipShape(Capsule())
            
            HStack(spacing: 4) {
                Image(systemName: "wrench.and.screwdriver")
                    .font(.system(size: 11))
                    .foregroundColor(.indigo)
                Text("\(totalTools) 个工具")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.secondary)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(Color(.secondarySystemBackground))
            .clipShape(Capsule())
            
            HStack(spacing: 4) {
                Image(systemName: "sparkles")
                    .font(.system(size: 11))
                    .foregroundColor(.purple)
                Text("\(skills.count) 个技能")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.secondary)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(Color(.secondarySystemBackground))
            .clipShape(Capsule())
            
            Spacer()
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
            errorMessage = "未连接网关"
            isLoading = false
            return
        }
        
        do {
            async let fetchMcp = APIClient.shared.fetchMcpServerStates(baseURL: url)
            async let fetchSkills = APIClient.shared.fetchAllSkills(baseURL: url)
            
            let (mcpResult, skillsResult) = try await (fetchMcp, fetchSkills)
            self.mcpServers = mcpResult
            self.skills = skillsResult
            // Expand all by default if there are fewer than 6 servers
            if mcpServers.count <= 6 {
                self.expandedServerNames = Set(mcpServers.map { $0.spec.serverName })
            }
        } catch {
            if settings.isDemoMode {
                applyDemoData()
            } else {
                errorMessage = error.localizedDescription
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
        self.expandedServerNames = Set(mcpServers.map { $0.spec.serverName })
    }
}

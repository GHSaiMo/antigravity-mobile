import Foundation

// MARK: - MCP Server Models

public struct McpServerSpec: Codable, Sendable, Hashable {
    public let serverName: String
    public let command: String?
    public let args: [String]?
    public let env: [String: String]?

    public init(serverName: String, command: String? = nil, args: [String]? = nil, env: [String: String]? = nil) {
        self.serverName = serverName
        self.command = command
        self.args = args
        self.env = env
    }
}

public struct McpServerInfoDetail: Codable, Sendable, Hashable {
    public let name: String?
    public let version: String?

    public init(name: String? = nil, version: String? = nil) {
        self.name = name
        self.version = version
    }
}

public struct McpToolParamInfo: Sendable, Hashable {
    public let name: String
    public let type: String
    public let description: String
    public let isRequired: Bool
    public let defaultValue: String?
}

public struct McpToolDefinition: Codable, Sendable, Identifiable, Hashable {
    public var id: String { "\(serverName ?? "")_\(name)" }
    public let name: String
    public let description: String?
    public let serverName: String?
    public let jsonSchemaString: String?

    public init(name: String, description: String? = nil, serverName: String? = nil, jsonSchemaString: String? = nil) {
        self.name = name
        self.description = description
        self.serverName = serverName
        self.jsonSchemaString = jsonSchemaString
    }

    /// 解析后的参数列表，供移动端参数卡片直接展示
    public var parsedParameters: [McpToolParamInfo] {
        guard let schemaStr = jsonSchemaString,
              let data = schemaStr.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return []
        }

        let requiredList = (json["required"] as? [String]) ?? []
        let properties = (json["properties"] as? [String: [String: Any]]) ?? [:]

        return properties.compactMap { key, val in
            let type = (val["type"] as? String) ?? "any"
            let desc = (val["description"] as? String) ?? ""
            let req = requiredList.contains(key)
            var defVal: String? = nil
            if let d = val["default"] {
                defVal = "\(d)"
            }
            return McpToolParamInfo(name: key, type: type, description: desc, isRequired: req, defaultValue: defVal)
        }.sorted { a, b in
            if a.isRequired != b.isRequired {
                return a.isRequired && !b.isRequired
            }
            return a.name < b.name
        }
    }
}

public struct McpServerState: Codable, Sendable, Identifiable, Hashable {
    public var id: String { spec.serverName }
    public let spec: McpServerSpec
    public let status: String
    public let tools: [McpToolDefinition]?
    public let serverInfo: McpServerInfoDetail?
    public let instructions: String?
    public let toolErrors: [String]?

    public init(
        spec: McpServerSpec,
        status: String,
        tools: [McpToolDefinition]? = nil,
        serverInfo: McpServerInfoDetail? = nil,
        instructions: String? = nil,
        toolErrors: [String]? = nil
    ) {
        self.spec = spec
        self.status = status
        self.tools = tools
        self.serverInfo = serverInfo
        self.instructions = instructions
        self.toolErrors = toolErrors
    }

    public var isReady: Bool {
        status.contains("READY")
    }

    public var statusTitle: String {
        if status.contains("READY") { return "就绪" }
        if status.contains("STARTING") { return "启动中" }
        if status.contains("ERROR") { return "异常" }
        if status.contains("STOPPED") { return "已停止" }
        return status
    }

    public var toolCount: Int {
        tools?.count ?? 0
    }
}

public struct GetMcpServerStatesResponse: Codable, Sendable {
    public let states: [McpServerState]?

    public init(states: [McpServerState]? = nil) {
        self.states = states
    }
}

// MARK: - Skills Models

public struct SkillInfo: Codable, Sendable, Identifiable, Hashable {
    public var id: String { name }
    public let name: String
    public let description: String?
    public let path: String?
    public let content: String?

    public init(name: String, description: String? = nil, path: String? = nil, content: String? = nil) {
        self.name = name
        self.description = description
        self.path = path
        self.content = content
    }

    public var isBuiltin: Bool {
        path?.contains("builtin") == true
    }

    public var locationCategory: String {
        if isBuiltin { return "内置技能" }
        if path?.contains(".gemini/config") == true { return "用户技能" }
        return "工作区技能"
    }
}

public struct GetAllSkillsResponse: Codable, Sendable {
    public let skills: [SkillInfo]?

    public init(skills: [SkillInfo]? = nil) {
        self.skills = skills
    }
}

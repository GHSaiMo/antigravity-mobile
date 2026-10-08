import SwiftUI

/// "投递到…" sheet shown when files are shared into the app from outside.
/// Files are never sent automatically: choosing a destination only drops them into that
/// conversation's input bar as attachments, so the user can add an instruction first.
struct ShareTargetSheet: View {
    @Bindable var inbox: ShareInbox
    let conversations: [ConversationItem]
    let currentConversationId: String?
    let onSelectProject: (ProjectItem) -> Void
    let onSelectConversation: (ConversationItem) -> Void
    
    private enum Tab: Hashable { case new, existing }
    @State private var tab: Tab
    @State private var query = ""
    @State private var projects: [ProjectItem] = ProjectCacheManager.shared.loadProjects()
    
    init(
        inbox: ShareInbox,
        conversations: [ConversationItem],
        currentConversationId: String?,
        onSelectProject: @escaping (ProjectItem) -> Void,
        onSelectConversation: @escaping (ConversationItem) -> Void
    ) {
        self.inbox = inbox
        self.conversations = conversations
        self.currentConversationId = currentConversationId
        self.onSelectProject = onSelectProject
        self.onSelectConversation = onSelectConversation
        _tab = State(initialValue: currentConversationId == nil ? .new : .existing)
    }
    
    private var deliverableCount: Int { inbox.files.filter { $0.isDeliverable }.count }
    
    private var filteredConversations: [ConversationItem] {
        let q = query.trimmingCharacters(in: .whitespaces).lowercased()
        return conversations
            .filter { !$0.isSubagent && $0.id != currentConversationId }
            .filter { q.isEmpty || $0.title.lowercased().contains(q) || $0.workspaceName.lowercased().contains(q) }
            .sorted { a, b in
                if (a.status == .running) != (b.status == .running) { return a.status == .running }
                return (a.lastModified ?? .distantPast) > (b.lastModified ?? .distantPast)
            }
            .prefix(50)
            .map { $0 }
    }
    
    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                fileList
                Picker("", selection: $tab) {
                    Text("新对话").tag(Tab.new)
                    Text("现有对话").tag(Tab.existing)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                
                if deliverableCount == 0 {
                    Text("没有可投递的文件")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if tab == .new {
                    newConversationList
                } else {
                    existingConversationList
                }
            }
            .navigationTitle("投递 \(deliverableCount) 个文件到…")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("全部丢弃", role: .destructive) { inbox.discardAll() }
                        .font(.system(size: 14))
                }
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .task {
            let fetched = await ProjectCacheManager.shared.fetchAndCacheProjects()
            if !fetched.isEmpty { projects = fetched }
        }
    }
    
    /// Single file is the common case, so each row is a tall card with a large icon; with more
    /// files the list simply grows downward (scrolling only once it gets long).
    @ViewBuilder
    private var fileList: some View {
        if inbox.files.count <= 3 {
            fileRows
        } else {
            ScrollView { fileRows }
                .scrollBounceBehavior(.basedOnSize)
                .frame(maxHeight: 320)
        }
    }
    
    private var fileRows: some View {
        VStack(spacing: 10) {
                ForEach(inbox.files) { f in
                    HStack(spacing: 14) {
                        FileTypeBadge(fileName: f.name, size: 56)
                        VStack(alignment: .leading, spacing: 4) {
                            Text(f.name).font(.system(size: 15, weight: .medium)).lineLimit(2)
                            Text(f.rejectReason ?? formatFileSize(f.size))
                                .font(.system(size: 12))
                                .foregroundColor(f.rejectReason == nil ? .secondary : .red)
                                .lineLimit(2)
                        }
                        Spacer(minLength: 4)
                        Button { inbox.remove(f.id) } label: {
                            Image(systemName: "xmark")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(.secondary)
                                .frame(width: 32, height: 32)
                        }
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 14)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                }
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
    }
    
    private var newConversationList: some View {
        ScrollView {
            LazyVStack(spacing: 8) {
                targetRow(icon: "bubble.left", title: "新对话", subtitle: "不关联任何工作区", tint: .indigo) {
                    onSelectProject(ProjectItem.pureChat)
                }
                ForEach(projects.filter { !$0.isPureChat }) { p in
                    targetRow(icon: "folder", title: p.alias?.isEmpty == false ? p.alias! : p.name, subtitle: p.path, tint: .blue) {
                        onSelectProject(p)
                    }
                }
            }
            .padding(16)
        }
    }
    
    private var existingConversationList: some View {
        VStack(spacing: 0) {
            HStack {
                Image(systemName: "magnifyingglass").foregroundColor(.secondary)
                TextField("搜索会话或工作区", text: $query)
                    .textFieldStyle(.plain)
            }
            .padding(10)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .padding(.horizontal, 16)
            ScrollView {
                LazyVStack(spacing: 8) {
                    if let cid = currentConversationId, query.isEmpty,
                       let current = conversations.first(where: { $0.id == cid }) {
                        targetRow(icon: "bubble.left.fill", title: "当前对话", subtitle: current.title, tint: .green) {
                            onSelectConversation(current)
                        }
                    }
                    ForEach(filteredConversations) { c in
                        targetRow(
                            icon: "bubble.left",
                            title: c.title,
                            subtitle: [c.workspaceName, c.status == .running ? "运行中" : nil]
                                .compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · "),
                            tint: .indigo
                        ) { onSelectConversation(c) }
                    }
                }
                .padding(16)
            }
        }
    }
    
    private func targetRow(icon: String, title: String, subtitle: String, tint: Color, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 14) {
                Image(systemName: icon)
                    .font(.system(size: 16))
                    .foregroundColor(tint)
                    .frame(width: 38, height: 38)
                    .background(tint.opacity(0.14))
                    .clipShape(Circle())
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(.system(size: 15, weight: .semibold)).foregroundColor(.primary).lineLimit(1)
                    if !subtitle.isEmpty {
                        Text(subtitle).font(.system(size: 12)).foregroundColor(.secondary).lineLimit(1)
                    }
                }
                Spacer()
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 11)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        }
        .buttonStyle(.plain)
    }
}

import 'dart:async';
import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, ChatStatus, Conversation, ChatMessage;

final _timeFmt = DateFormat('HH:mm');

class SimplexChatScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const SimplexChatScreen({
    super.key,
    required this.client,
    required this.identity,
  });

  @override
  State<SimplexChatScreen> createState() => _SimplexChatScreenState();
}

class _SimplexChatScreenState extends State<SimplexChatScreen> with WidgetsBindingObserver {
  List<ChatMessage> _messages = [];
  List<Conversation> _conversations = [];
  final _inputCtrl = TextEditingController();
  final _connectCtrl = TextEditingController();
  final _searchCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();
  bool _loading = true;
  String _selectedConversationId = '';
  String _contactLink = '';
  Timer? _pollTimer;
  int _selectedTab = 0;
  String _bridgeStatus = 'unknown';

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _load();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _inputCtrl.dispose();
    _connectCtrl.dispose();
    _searchCtrl.dispose();
    _scrollCtrl.dispose();
    _pollTimer?.cancel();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused) {
      _pollTimer?.cancel();
    } else if (state == AppLifecycleState.resumed) {
      _load();
    }
  }

  Future<void> _load() async {
    if (!mounted) return;
    setState(() => _loading = true);
    try {
      final results = await Future.wait([
        widget.client.chat.status(),
        widget.client.chat.conversations(pubkey: widget.identity.ed25519PubKey),
      ]);
      final status = results[0] as ChatStatus;
      final conversations = results[1] as List<Conversation>;
      if (mounted) {
        setState(() {
          _bridgeStatus = status.bridgeStatus;
          _conversations = conversations;
          _loading = false;
        });
        if (conversations.isNotEmpty && _selectedConversationId.isEmpty) {
          _selectConversation(conversations.first.id);
        }
      }
    } catch (e) {
      if (mounted) {
        setState(() => _loading = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Error loading chat: $e'), backgroundColor: Colors.red),
        );
      }
    }
  }

  void _selectConversation(String conversationId) {
    setState(() {
      _selectedConversationId = conversationId;
      _messages.clear();
    });
    _loadMessages(conversationId);
    _startSse(conversationId);
  }

  Future<void> _loadMessages(String conversationId) async {
    try {
      setState(() {});
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Error loading messages: $e'), backgroundColor: Colors.red),
        );
      }
    }
  }

  void _startSse(String conversationId) {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 30), (_) {
      if (mounted && _selectedConversationId == conversationId) {
        _loadMessages(conversationId);
      }
    });
  }

  Future<void> _sendMessage() async {
    final text = _inputCtrl.text.trim();
    if (text.isEmpty || _selectedConversationId.isEmpty) return;
    _inputCtrl.clear();
    try {
      await widget.client.chat.send(
        to: _selectedConversationId,
        from: widget.identity.ed25519PubKey,
        message: text,
      );
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Send failed: $e'), backgroundColor: Colors.red),
        );
      }
    }
  }

  Future<void> _createConversation() async {
    final nameCtrl = TextEditingController();
    final descCtrl = TextEditingController();
    final participantsCtrl = TextEditingController();

    final result = await showDialog<Map<String, String>>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('New Conversation'),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: nameCtrl, decoration: const InputDecoration(labelText: 'Name', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: descCtrl, decoration: const InputDecoration(labelText: 'Description', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: participantsCtrl, decoration: const InputDecoration(labelText: 'Participants (comma-separated pubkeys)', border: OutlineInputBorder())),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.of(ctx).pop({'name': nameCtrl.text, 'description': descCtrl.text, 'participants': participantsCtrl.text}), child: const Text('Create')),
        ],
      ),
    );

    if (result != null && result['name']!.isNotEmpty) {
      _load();
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }

    final theme = Theme.of(context);
    final selectedConv = _conversations.firstWhere(
      (c) => c.id == _selectedConversationId,
      orElse: () => _conversations.isNotEmpty ? _conversations.first : Conversation(id: '', name: '', description: '', participants: [], createdBy: '', createdAt: 0),
    );

    return Scaffold(
      appBar: AppBar(
        title: Text(selectedConv.name.isNotEmpty ? selectedConv.name : 'Chat'),
        actions: [
          if (_selectedTab == 0) ...[
            IconButton(icon: const Icon(Icons.add), onPressed: _createConversation, tooltip: 'New Conversation'),
            IconButton(icon: const Icon(Icons.refresh), onPressed: _load, tooltip: 'Refresh'),
          ],
          PopupMenuButton<int>(
            onSelected: (v) => setState(() => _selectedTab = v),
            itemBuilder: (ctx) => [
              const PopupMenuItem(value: 0, child: Row(children: [Icon(Icons.chat), SizedBox(width: 8), Text('Messages')])),
              const PopupMenuItem(value: 1, child: Row(children: [Icon(Icons.people), SizedBox(width: 8), Text('Contacts')])),
              const PopupMenuItem(value: 2, child: Row(children: [Icon(Icons.settings), SizedBox(width: 8), Text('Settings')])),
            ],
          ),
        ],
        bottom: _selectedTab == 0
            ? null
            : PreferredSize(
                preferredSize: const Size.fromHeight(56),
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                  child: TextField(
                    controller: _searchCtrl,
                    decoration: InputDecoration(
                      hintText: _selectedTab == 1 ? 'Search contacts...' : 'Search...',
                      prefixIcon: const Icon(Icons.search),
                      border: OutlineInputBorder(borderRadius: BorderRadius.circular(24)),
                      filled: true,
                      contentPadding: const EdgeInsets.symmetric(horizontal: 16),
                    ),
                    onChanged: (v) => setState(() {}),
                  ),
                ),
              ),
      ),
      body: _selectedTab == 0
          ? _buildChatView(theme, selectedConv)
          : _selectedTab == 1
              ? ListView.builder(
                  padding: const EdgeInsets.all(16),
                  itemCount: _conversations.length,
                  itemBuilder: (ctx, i) {
                    final conv = _conversations[i];
                    return ListTile(
                      leading: CircleAvatar(child: Text(conv.name.isNotEmpty ? conv.name[0] : '?')),
                      title: Text(conv.name),
                      subtitle: Text('${conv.participants.length} participants'),
                      trailing: IconButton(icon: const Icon(Icons.chevron_right), onPressed: () => _selectConversation(conv.id)),
                      onTap: () => _selectConversation(conv.id),
                    );
                  },
                )
              : _buildSettingsTab(theme),
    );
  }

  Widget _buildChatView(ThemeData theme, Conversation? selectedConv) {
    return Column(
      children: [
        if (_selectedConversationId.isEmpty)
          Expanded(
            child: Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.chat_bubble_outline, size: 64, color: theme.colorScheme.primary.withValues(alpha: 0.5)),
                  const SizedBox(height: 16),
                  Text('Select a conversation or create a new one', style: theme.textTheme.titleMedium?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
                  const SizedBox(height: 16),
                  FilledButton.icon(onPressed: _createConversation, icon: const Icon(Icons.add), label: const Text('New Conversation')),
                ],
              ),
            ),
          )
        else
          Expanded(
              child: ListView.builder(
                controller: _scrollCtrl,
                padding: const EdgeInsets.all(16),
                itemCount: _messages.length,
                itemBuilder: (ctx, i) {
                  final msg = _messages[i];
                  final isUser = msg.from == widget.identity.ed25519PubKey;
                  return Align(
                    alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
                    child: Container(
                      margin: const EdgeInsets.symmetric(vertical: 4),
                      padding: const EdgeInsets.all(12),
                      constraints: BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.75),
                      decoration: BoxDecoration(
                        color: isUser ? theme.colorScheme.primary : theme.colorScheme.surfaceContainerHighest,
                        borderRadius: BorderRadius.circular(16),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(msg.text, style: TextStyle(color: isUser ? theme.colorScheme.onPrimary : theme.colorScheme.onSurface)),
                          const SizedBox(height: 4),
                          Text(_timeFmt.format(DateTime.tryParse(msg.timestamp) ?? DateTime.now()), style: theme.textTheme.bodySmall?.copyWith(color: (isUser ? theme.colorScheme.onPrimary : theme.colorScheme.onSurfaceVariant).withValues(alpha: 0.7))),
                        ],
                      ),
                    ),
                  );
                },
              ),
            ),
        if (_selectedConversationId.isNotEmpty)
          SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _inputCtrl,
                      decoration: const InputDecoration(hintText: 'Message...', border: OutlineInputBorder()),
                      onSubmitted: (_) => _sendMessage(),
                    ),
                  ),
                  const SizedBox(width: 8),
                  IconButton(
                    icon: const Icon(Icons.send),
                    onPressed: _sendMessage,
                    style: IconButton.styleFrom(backgroundColor: theme.colorScheme.primary, foregroundColor: theme.colorScheme.onPrimary),
                  ),
                ],
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildSettingsTab(ThemeData theme) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Connection', style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)),
                const SizedBox(height: 12),
                ListTile(
                  leading: Icon(_bridgeStatus == 'connected' ? Icons.check_circle : Icons.error, color: _bridgeStatus == 'connected' ? Colors.green : Colors.red),
                  title: const Text('Bridge Status'),
                  subtitle: Text(_bridgeStatus),
                ),
                ListTile(
                  leading: const Icon(Icons.link),
                  title: const Text('Your Invite Link'),
                  subtitle: Text(_contactLink.isEmpty ? 'Not connected' : _contactLink),
                  trailing: _contactLink.isNotEmpty ? IconButton(icon: const Icon(Icons.copy), onPressed: () {}) : null,
                ),
                ListTile(
                  leading: const Icon(Icons.person_add),
                  title: const Text('Connect via Link'),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: _showConnectDialog,
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 16),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Privacy', style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)),
                const SizedBox(height: 12),
                SwitchListTile(
                  title: const Text('Enable Encryption'),
                  subtitle: const Text('End-to-end encryption for messages'),
                  value: true,
                  onChanged: (v) {},
                ),
                SwitchListTile(
                  title: const Text('Auto-delete Messages'),
                  subtitle: const Text('Delete messages after reading'),
                  value: false,
                  onChanged: (v) {},
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  void _showConnectDialog() {
    final ctrl = TextEditingController();
    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Connect via Link'),
        content: TextField(controller: ctrl, decoration: const InputDecoration(labelText: 'Invite Link', border: OutlineInputBorder())),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(onPressed: () { Navigator.pop(ctx); }, child: const Text('Connect')),
        ],
      ),
    );
  }
}
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../theme.dart';
import '../main.dart';

/// AIOfficeScreen manages aiofficescreen functionality.
class AIOfficeScreen extends StatefulWidget {
  const AIOfficeScreen({super.key});
  @override
  State<AIOfficeScreen> createState() => _AIOfficeScreenState();
}

class _AIOfficeScreenState extends State<AIOfficeScreen> {
  final _chatController = TextEditingController();
  final _scrollController = ScrollController();
  final List<ChatMessage> _messages = [];
  bool _loading = false;

  static const _suggestions = [
    'Show me the treasury status',
    'What is the silver reserve?',
    'Execute a health check of all services',
    'Explain the silver standard',
    'Trigger a deflation check',
    'Show recent audit log entries',
    'What is our node reputation?',
    'Broadcast a message to all contacts',
  ];

  @override
  void initState() {
    super.initState();
    _messages.add(ChatMessage(
      'Greetings, Regent. I am the Island Steward — your AI executive '
      'assistant for the Saint Mary Liberty Island sovereign network. '
      'How may I serve the Crown today?',
      false,
    ));
  }

  @override
  void dispose() {
    _chatController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  Future<void> _send(String text) async {
    if (text.trim().isEmpty) return;
    setState(() {
      _messages.add(ChatMessage(text, true));
      _loading = true;
    });
    _chatController.clear();
    _scrollToBottom();

    try {
      final api = context.read<AppState>().api;
      final res = await api.aiChat(text, profile: 'steward');
      final answer = res['answer'] as String? ?? 'No response from Steward.';
      if (mounted) {
        setState(() {
          _messages.add(ChatMessage(answer, false));
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _messages.add(ChatMessage('Error: ${e.toString()}', false));
          _loading = false;
        });
      }
    }
    _scrollToBottom();
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.animateTo(
          _scrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 300),
          curve: Curves.easeOut,
        );
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Row(children: [
          Container(
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              color: RoyalTheme.gold.withAlpha(30),
              borderRadius: BorderRadius.circular(8),
            ),
            child: const Icon(Icons.auto_awesome, color: RoyalTheme.gold, size: 20),
          ),
          const SizedBox(width: 12),
          const Text('AI Executive Office'),
        ]),
      ),
      body: Column(children: [
        if (_messages.length <= 1)
          _buildSuggestions(),
        Expanded(
          child: ListView.builder(
            controller: _scrollController,
            padding: const EdgeInsets.all(16),
            itemCount: _messages.length,
            itemBuilder: (context, i) => _buildMessage(_messages[i]),
          ),
        ),
        if (_loading)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
              const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: RoyalTheme.gold)),
              const SizedBox(width: 8),
              Text('Steward is thinking...', style: Theme.of(context).textTheme.bodyMedium),
            ]),
          ),
        _buildInput(),
      ]),
    );
  }

  Widget _buildSuggestions() {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Padding(
          padding: const EdgeInsets.only(left: 4, bottom: 8),
          child: Text('Suggestions', style: Theme.of(context).textTheme.titleMedium),
        ),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: _suggestions.map((s) => ActionChip(
            label: Text(s, style: const TextStyle(fontSize: 14)),
            backgroundColor: RoyalTheme.darkCard,
            side: const BorderSide(color: Color(0xFF30363D)),
            onPressed: () => _send(s),
          )).toList(),
        ),
      ]),
    );
  }

  Widget _buildMessage(ChatMessage msg) {
    final isUser = msg.isUser;
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Row(
        mainAxisAlignment: isUser ? MainAxisAlignment.end : MainAxisAlignment.start,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (!isUser) ...[
            Container(
              width: 32, height: 32,
              decoration: const BoxDecoration(
                shape: BoxShape.circle,
                gradient: LinearGradient(
                  colors: [RoyalTheme.gold, Color(0xFFFFA500)],
                ),
              ),
              child: const Center(child: Text('♚', style: TextStyle(fontSize: 16, color: Colors.black))),
            ),
            const SizedBox(width: 8),
          ],
          Flexible(
            child: Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: isUser ? RoyalTheme.gold.withAlpha(20) : RoyalTheme.darkCard,
                borderRadius: BorderRadius.circular(16).copyWith(
                  bottomLeft: isUser ? null : const Radius.circular(4),
                  bottomRight: isUser ? const Radius.circular(4) : null,
                ),
                border: Border.all(color: isUser ? RoyalTheme.gold.withAlpha(40) : const Color(0xFF30363D)),
              ),
              child: Text(
                msg.text,
                style: TextStyle(
                  color: isUser ? RoyalTheme.gold : Colors.white,
                  fontSize: 15,
                  height: 1.5,
                ),
              ),
            ),
          ),
          if (isUser) ...[
            const SizedBox(width: 8),
            Container(
              width: 32, height: 32,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: RoyalTheme.accent.withAlpha(40),
              ),
              child: const Center(child: Icon(Icons.person, size: 18, color: RoyalTheme.accent)),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildInput() {
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      decoration: const BoxDecoration(
        color: RoyalTheme.darkCard,
        border: Border(top: BorderSide(color: Color(0xFF21262D))),
      ),
      child: Row(children: [
        Expanded(
          child: TextField(
            controller: _chatController,
            decoration: const InputDecoration(
              hintText: 'Command the Steward...',
              border: InputBorder.none,
              filled: false,
            ),
            style: const TextStyle(color: Colors.white, fontSize: 14),
            textInputAction: TextInputAction.send,
            onSubmitted: _send,
          ),
        ),
        const SizedBox(width: 8),
        Container(
          decoration: BoxDecoration(
            color: RoyalTheme.gold,
            borderRadius: BorderRadius.circular(12),
          ),
          child: IconButton(
            icon: const Icon(Icons.send_rounded, color: Colors.black),
            onPressed: () => _send(_chatController.text),
          ),
        ),
      ]),
    );
  }
}

/// ChatMessage manages data model for a single chat message with all metadata.
class ChatMessage {
  final String text;
  final bool isUser;
  ChatMessage(this.text, this.isUser);
}

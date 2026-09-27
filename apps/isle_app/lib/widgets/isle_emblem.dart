import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'dart:convert';

/// A round emblem using the original stmaria.org coat of arms
/// with a gold circular border. Clickable → opens declaration + Steward AI chat.
/// RoundEmblem manages a circular emblem displaying the island coat of arms.
class RoundEmblem extends StatelessWidget {
  final double size;
  final VoidCallback? onTap;

  const RoundEmblem({super.key, this.size = 150, this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: SizedBox(
        width: size,
        height: size,
        child: ClipOval(
          child: Image.asset('assets/images/emblem.png', width: size, height: size, fit: BoxFit.cover),
        ),
      ),
    );
  }
}

/// Shows the full project declaration in a dialog,
/// with a link to chat with Steward AI Bot via Simplex.
void showIsleDeclaration(BuildContext context, {http.Client? httpClient, String? serverUrl, String? pubkey}) {
  showDialog(
    context: context,
    builder: (ctx) => _DeclarationDialog(httpClient: httpClient, serverUrl: serverUrl, pubkey: pubkey),
  );
}

class _DeclarationDialog extends StatefulWidget {
  final http.Client? httpClient;
  final String? serverUrl;
  final String? pubkey;

  const _DeclarationDialog({this.httpClient, this.serverUrl, this.pubkey});

  @override
  State<_DeclarationDialog> createState() => _DeclarationDialogState();
}

class _DeclarationDialogState extends State<_DeclarationDialog> {
  @override
  Widget build(BuildContext context) {
    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      insetPadding: const EdgeInsets.symmetric(horizontal: 24, vertical: 32),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 520, maxHeight: 620),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // Header
            Container(
              padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 16),
              decoration: const BoxDecoration(
                border: Border(bottom: BorderSide(color: Color(0xFFFFD700), width: 0.5)),
              ),
              child: Row(
                children: [
                  const Icon(Icons.menu_book, size: 18, color: Color(0xFFFFD700)),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text('THE ISLAND — SAINT MARY LIBERTY ISLAND',
                      style: TextStyle(fontSize: 11, fontWeight: FontWeight.bold, color: const Color(0xFFFFD700), letterSpacing: 1.2),
                    ),
                  ),
                  IconButton(
                    icon: const Icon(Icons.close, size: 16, color: Colors.grey),
                    onPressed: () => Navigator.pop(context),
                    padding: EdgeInsets.zero, constraints: const BoxConstraints(),
                  ),
                ],
              ),
            ),
            // Scrollable content
            Flexible(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _section('DECLARATION OF SOVEREIGNTY',
                      'The Island is a self-sovereign digital commonwealth built on the '
                          'principles of individual liberty, privacy, and voluntary association. '
                          'It is not a company, not a state, not a platform — it is a sovereign '
                          'network owned by its users.'),
                    const SizedBox(height: 12),
                    _principle('01', 'SOVEREIGNTY',
                      'Every user owns their identity, keys, and assets. No third party can seize, freeze, or control them.'),
                    _principle('02', 'PRIVACY',
                      'All communications are routed through the Tor network. No metadata is logged. No tracking.'),
                    _principle('03', 'SELF-GOVERNANCE',
                      'The Island is governed by its users through DAO proposals and voting.'),
                    _principle('04', 'SOVEREIGN ECONOMY',
                      'Silver banknotes and Liquid Taler form a censorship-resistant monetary system.'),
                    _principle('05', 'FREE SPEECH',
                      'Radio broadcasting and messaging are uncensored, with infrastructure but no editorial control.'),
                    _principle('06', 'ENCRYPTION',
                      'All data is encrypted at rest and in transit. PIN-based access, no password reset.'),
                    _principle('07', 'OPEN SOURCE',
                      'The code is transparent, auditable, and built by the community.'),
                    const SizedBox(height: 10),
                    Divider(color: Colors.grey.shade700, height: 1),
                    const SizedBox(height: 8),
                    Text(
                      'An extension of the Saint Mary Liberty Island vision — digital sovereignty meets practical technology. No KYC, no surveillance, no permission required.',
                      style: TextStyle(fontSize: 10, color: Colors.grey[400], height: 1.5, fontStyle: FontStyle.italic),
                    ),
                    const SizedBox(height: 6),
                    Text('stmaria.org', style: TextStyle(fontSize: 11, color: const Color(0xFFFFD700), fontWeight: FontWeight.bold, letterSpacing: 2)),
                    const SizedBox(height: 12),
                    SizedBox(
                      width: double.infinity,
                      child: OutlinedButton.icon(
                        icon: const Icon(Icons.smart_toy, size: 16),
                        label: const Text('Chat with Steward AI', style: TextStyle(fontSize: 12)),
                        style: OutlinedButton.styleFrom(
                          foregroundColor: const Color(0xFFFFD700),
                          side: const BorderSide(color: Color(0xFFFFD700), width: 0.5),
                          padding: const EdgeInsets.symmetric(vertical: 8),
                        ),
                        onPressed: () => _openStewardChat(context),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  void _openStewardChat(BuildContext context) {
    Navigator.pop(context);
    showDialog(
      context: context,
      builder: (ctx) => _StewardChatDialog(
        httpClient: widget.httpClient,
        serverUrl: widget.serverUrl,
        pubkey: widget.pubkey,
      ),
    );
  }

  Widget _section(String title, String body) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title, style: TextStyle(fontSize: 10, color: const Color(0xFFFFD700), fontWeight: FontWeight.w600, letterSpacing: 1.2)),
        const SizedBox(height: 4),
        Text(body, style: TextStyle(fontSize: 11, color: Colors.grey[300], height: 1.5)),
      ],
    );
  }

  Widget _principle(String num, String title, String body) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 22,
            child: Text(num, style: TextStyle(fontSize: 9, color: const Color(0xFFFFD700).withValues(alpha: 0.6), fontWeight: FontWeight.bold)),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: TextStyle(fontSize: 11, color: const Color(0xFFFFD700), fontWeight: FontWeight.w600, letterSpacing: 0.8)),
                const SizedBox(height: 1),
                Text(body, style: TextStyle(fontSize: 10, color: Colors.grey[400], height: 1.4)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// A simple chat dialog to talk to the Steward AI Bot.
class _StewardChatDialog extends StatefulWidget {
  final http.Client? httpClient;
  final String? serverUrl;
  final String? pubkey;

  const _StewardChatDialog({this.httpClient, this.serverUrl, this.pubkey});

  @override
  State<_StewardChatDialog> createState() => _StewardChatDialogState();
}

class _StewardChatDialogState extends State<_StewardChatDialog> {
  final _messages = <_ChatMessage>[
    _ChatMessage('Welcome, citizen. I am the AI Steward of Saint Mary Liberty Island. Ask me anything about the Island — its economy, governance, or way of life.', false),
  ];
  final _inputCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();
  bool _sending = false;

  @override
  void dispose() {
    _inputCtrl.dispose();
    _scrollCtrl.dispose();
    super.dispose();
  }

  void _scrollDown() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollCtrl.hasClients) _scrollCtrl.animateTo(_scrollCtrl.position.maxScrollExtent, duration: const Duration(milliseconds: 200), curve: Curves.easeOut);
    });
  }

  Future<void> _sendMessage() async {
    final text = _inputCtrl.text.trim();
    if (text.isEmpty) return;
    setState(() {
      _messages.add(_ChatMessage(text, true));
      _sending = true;
      _inputCtrl.clear();
    });
    _scrollDown();

    try {
      final client = widget.httpClient ?? http.Client();
      final baseUrl = widget.serverUrl ?? 'http://127.0.0.1:8080';

      final envelope = {
        'type': 'api.request',
        'payload': {
          'method': 'POST',
          'path': '/api/ai/chat',
          'body': {'question': text, 'context': ''},
        },
        'id': 'steward-chat-${DateTime.now().millisecondsSinceEpoch}',
      };

      final transportUrl = Uri.tryParse(baseUrl)?.resolve('/api/transport/send') ?? Uri.parse('$baseUrl/api/transport/send');
      final resp = await client.post(
        transportUrl,
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode(envelope),
      ).timeout(const Duration(seconds: 30));

      String reply;
      if (resp.statusCode == 200) {
        final data = jsonDecode(resp.body) as Map<String, dynamic>;
        if (data['type'] == 'response') {
          final payload = data['payload'];
          if (payload is Map) {
            final body = payload['body'];
            if (body is Map && body['answer'] != null) {
              reply = body['answer'] as String;
            } else if (body is Map && body['response'] != null) {
              reply = body['response'] as String;
            } else {
              reply = 'The Steward considered your question but gave no clear answer.';
            }
          } else {
            reply = 'Unexpected response format from Steward.';
          }
        } else if (data['type'] == 'error') {
          final errPayload = data['payload'];
          reply = 'Steward error: ${errPayload is Map ? errPayload['error'] ?? 'unknown' : 'unknown'}';
        } else {
          reply = 'Unexpected response type: ${data['type']}';
        }
      } else {
        reply = 'The Steward is currently in contemplation. Please try again later. (Error: ${resp.statusCode})';
      }

      if (mounted) {
        setState(() {
          _messages.add(_ChatMessage(reply, false));
          _sending = false;
        });
        _scrollDown();
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _messages.add(_ChatMessage('The connection to the Steward faltered. The Island network may be experiencing turbulence. ($e)', false));
          _sending = false;
        });
        _scrollDown();
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Dialog(
      backgroundColor: const Color(0xFF1A1A2E),
      insetPadding: const EdgeInsets.symmetric(horizontal: 20, vertical: 32),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 440, maxHeight: 480),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 16),
              decoration: const BoxDecoration(
                border: Border(bottom: BorderSide(color: Color(0xFFFFD700), width: 0.5)),
              ),
              child: Row(
                children: [
                  const Icon(Icons.smart_toy, size: 18, color: Color(0xFFFFD700)),
                  const SizedBox(width: 6),
                  const Expanded(child: Text('Steward AI Bot', style: TextStyle(fontSize: 12, fontWeight: FontWeight.bold, color: Color(0xFFFFD700), letterSpacing: 1))),
                  IconButton(
                    icon: const Icon(Icons.close, size: 16, color: Colors.grey),
                    onPressed: () => Navigator.pop(context),
                    padding: EdgeInsets.zero, constraints: const BoxConstraints(),
                  ),
                ],
              ),
            ),
            Flexible(
              child: _messages.isEmpty
                  ? const Center(child: Text('No messages', style: TextStyle(color: Colors.grey)))
                  : ListView.builder(
                      controller: _scrollCtrl,
                      padding: const EdgeInsets.all(12),
                      itemCount: _messages.length,
                      itemBuilder: (ctx, i) {
                        final msg = _messages[i];
                        return _chatBubble(msg);
                      },
                    ),
            ),
            if (_sending)
              const Padding(
                padding: EdgeInsets.only(bottom: 4),
                child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Color(0xFFFFD700))),
              ),
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                border: Border(top: BorderSide(color: Colors.grey.shade800)),
              ),
              child: Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _inputCtrl,
                      style: const TextStyle(fontSize: 12, color: Colors.white),
                      decoration: const InputDecoration(
                        hintText: 'Ask the Steward...',
                        hintStyle: TextStyle(fontSize: 12, color: Colors.grey),
                        border: InputBorder.none,
                        isDense: true,
                        contentPadding: EdgeInsets.symmetric(horizontal: 8, vertical: 6),
                      ),
                      onSubmitted: _sending ? null : (_) => _sendMessage(),
                    ),
                  ),
                  IconButton(
                    icon: const Icon(Icons.send, size: 18, color: Color(0xFFFFD700)),
                    onPressed: _sending ? null : _sendMessage,
                    padding: EdgeInsets.zero, constraints: const BoxConstraints(),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _chatBubble(_ChatMessage msg) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        mainAxisAlignment: msg.isUser ? MainAxisAlignment.end : MainAxisAlignment.start,
        children: [
          if (!msg.isUser) const Padding(
            padding: EdgeInsets.only(right: 6),
            child: Icon(Icons.smart_toy, size: 14, color: Color(0xFFFFD700)),
          ),
          Flexible(
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
              decoration: BoxDecoration(
                color: msg.isUser
                    ? const Color(0xFF2E7D32).withValues(alpha: 0.3)
                    : Colors.grey.shade800,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Text(msg.text, style: TextStyle(fontSize: 11, color: msg.isUser ? Colors.greenAccent : Colors.grey[300], height: 1.4)),
            ),
          ),
          if (msg.isUser) const Padding(
            padding: EdgeInsets.only(left: 6),
            child: Icon(Icons.person, size: 14, color: Colors.greenAccent),
          ),
        ],
      ),
    );
  }
}

class _ChatMessage {
  final String text;
  final bool isUser;
  _ChatMessage(this.text, this.isUser);
}

import 'dart:convert';
import 'package:flutter/material.dart';
import '../models/chat_message.dart';
import 'pulse_dot.dart';

/// ChatMessageBubble manages a single chat message bubble with encryption, reactions and status indicators.
class ChatMessageBubble extends StatelessWidget {
  final ChatMessage msg;
  final bool isUser;
  final bool bulkMode;
  final bool selected;
  final String? trustLevel;
  final String? statusText;
  final VoidCallback? onTap;
  final VoidCallback? onLongPress;
  final ValueChanged<String>? onReact;
  final VoidCallback? onEdit;
  final VoidCallback? onDelete;
  final VoidCallback? onForward;
  final VoidCallback? onPin;

  const ChatMessageBubble({
    super.key,
    required this.msg,
    required this.isUser,
    this.bulkMode = false,
    this.selected = false,
    this.trustLevel,
    this.statusText,
    this.onTap,
    this.onLongPress,
    this.onReact,
    this.onEdit,
    this.onDelete,
    this.onForward,
    this.onPin,
  });

  @override
  Widget build(BuildContext context) {
    final bubbleColor = isUser ? Colors.blue.shade800 : Colors.grey.shade800;
    final alignment = isUser ? CrossAxisAlignment.end : CrossAxisAlignment.start;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      child: Column(crossAxisAlignment: alignment, children: [
        if (msg.replyToId != null && msg.replyText != null)
          Container(
            margin: const EdgeInsets.only(bottom: 2),
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              color: Colors.grey.shade700.withValues(alpha: 0.5),
              borderRadius: BorderRadius.circular(6),
            ),
            child: Text('↩ ${msg.replyText!}', style: TextStyle(fontSize: 13, color: Colors.grey[400]), maxLines: 1, overflow: TextOverflow.ellipsis),
          ),
        Row(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.end, children: [
          if (!isUser) ...[
            _encryptionIcon(),
            const SizedBox(width: 4),
          ],
          if (bulkMode)
            Padding(
              padding: const EdgeInsets.only(right: 4),
              child: Icon(selected ? Icons.check_box : Icons.check_box_outline_blank, size: 20, color: Colors.white),
            ),
          Flexible(
            child: GestureDetector(
              onTap: onTap,
              onLongPress: onLongPress,
              child: Container(
                constraints: BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.72),
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                decoration: BoxDecoration(
                  color: bubbleColor,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  if (msg.isForwarded)
                    Row(children: [
                      Icon(Icons.redo, size: 12, color: Colors.grey[400]),
                      const SizedBox(width: 4),
                      Text('Forwarded', style: TextStyle(fontSize: 11, color: Colors.grey[400])),
                    ]),
                  if (msg.pinned != null)
                    Row(children: [
                      Icon(Icons.push_pin, size: 12, color: Colors.yellow[700]),
                      const SizedBox(width: 4),
                      Text('Pinned', style: TextStyle(fontSize: 11, color: Colors.yellow[700])),
                    ]),
                  if (msg.recalled)
                    Text('Message recalled', style: TextStyle(fontSize: 15, color: Colors.grey[500], fontStyle: FontStyle.italic))
                  else ...[
                    Text(msg.text, style: const TextStyle(fontSize: 17, color: Colors.white)),
                    if (msg.voiceUrl != null)
                      Row(children: [
                        Icon(Icons.mic, size: 16, color: Colors.grey[400]),
                        const SizedBox(width: 4),
                        Text('Voice ${msg.voiceDuration ?? 0}s', style: TextStyle(fontSize: 14, color: Colors.grey[400])),
                      ]),
                    if (msg.fileName != null)
                      Row(children: [
                        Icon(Icons.attach_file, size: 16, color: Colors.grey[400]),
                        const SizedBox(width: 4),
                        Flexible(child: Text(msg.fileName!, style: TextStyle(fontSize: 14, color: Colors.cyan[300]), overflow: TextOverflow.ellipsis)),
                      ]),
                    if (msg.moneyAmount != null)
                      Container(
                        margin: const EdgeInsets.only(top: 4),
                        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                        decoration: BoxDecoration(color: Colors.green.shade900, borderRadius: BorderRadius.circular(6)),
                        child: Row(mainAxisSize: MainAxisSize.min, children: [
                          Icon(Icons.payments, size: 14, color: Colors.green[300]),
                          const SizedBox(width: 4),
                          Text('${msg.moneyAmount!.toStringAsFixed(2)} ${msg.moneyAsset ?? "XAG"}', style: TextStyle(fontSize: 14, color: Colors.green[300], fontWeight: FontWeight.bold)),
                        ]),
                      ),
                  ],
                  const SizedBox(height: 4),
                  Row(mainAxisSize: MainAxisSize.min, children: [
                    Text(msg.displayTime, style: TextStyle(fontSize: 11, color: Colors.grey[500])),
                    if (msg.status == 'sending')
                      Padding(padding: const EdgeInsets.only(left: 4), child: SizedBox(width: 10, height: 10, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.grey[500]))),
                    if (msg.status == 'sent') Icon(Icons.check, size: 14, color: Colors.grey[500]),
                    if (msg.status == 'delivered') Icon(Icons.done_all, size: 14, color: Colors.blue[300]),
                    if (msg.status == 'failed') Icon(Icons.error, size: 14, color: Colors.red),
                    if (msg.readAt != null && msg.readAt!.isNotEmpty) Icon(Icons.visibility, size: 14, color: Colors.blue[300]),
                  ]),
                  if (msg.reactions != null && msg.reactions!.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(top: 4),
                      child: Wrap(spacing: 4, children: msg.reactions!.entries.map((e) => Container(
                        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                        decoration: BoxDecoration(color: Colors.grey.shade700, borderRadius: BorderRadius.circular(10)),
                        child: Text('${e.key} ${e.value}', style: const TextStyle(fontSize: 14)),
                      )).toList()),
                    ),
                  if (trustLevel != null && trustLevel != 'none')
                    Padding(
                      padding: const EdgeInsets.only(top: 2),
                      child: Row(children: [
                        Icon(trustLevel == 'verified' ? Icons.verified : Icons.shield, size: 12, color: trustLevel == 'verified' ? Colors.blue : Colors.green),
                        const SizedBox(width: 3),
                        Text(trustLevel!, style: TextStyle(fontSize: 10, color: Colors.grey[500])),
                      ]),
                    ),
                ]),
              ),
            ),
          ),
          if (isUser) ...[
            const SizedBox(width: 4),
            _encryptionIcon(),
          ],
        ]),
      ]),
    );
  }

  Widget _encryptionIcon() {
    if (msg.encryption == 'e2e') {
      return Icon(Icons.lock, size: 14, color: Colors.green[400]);
    } else if (msg.encryption == 'none') {
      return Icon(Icons.lock_open, size: 14, color: Colors.red[400]);
    }
    return const SizedBox(width: 14);
  }
}

// _PulseDot moved to pulse_dot.dart (shared)

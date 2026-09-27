import 'package:flutter/material.dart';

/// EncryptionIndicator manages a visual indicator showing whether a chat is end-to-end encrypted.
class EncryptionIndicator extends StatelessWidget {
  final String? encryption;
  final double size;

  const EncryptionIndicator({super.key, this.encryption, this.size = 14});

  @override
  Widget build(BuildContext context) {
    if (encryption == null || encryption == 'unknown') {
      return const SizedBox(width: 14);
    }
    final isE2E = encryption == 'e2e';
    return Tooltip(
      message: isE2E ? 'End-to-end encrypted' : 'Not encrypted',
      child: Icon(
        isE2E ? Icons.lock : Icons.lock_open,
        size: size,
        color: isE2E ? Colors.green[400] : Colors.red[400],
      ),
    );
  }
}

/// E2EBadge manages a compact badge showing E2E encryption status.
class E2EBadge extends StatelessWidget {
  final bool active;

  const E2EBadge({super.key, this.active = false});

  @override
  Widget build(BuildContext context) {
    if (!active) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: Colors.green.shade900,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.green.shade700, width: 1),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.lock, size: 12, color: Colors.green[300]),
          const SizedBox(width: 3),
          Text('e2e', style: TextStyle(fontSize: 11, color: Colors.green[300], fontWeight: FontWeight.bold)),
        ],
      ),
    );
  }
}

/// ChatEncryptionIndicator manages a visual indicator for chat encryption status.
class ChatEncryptionIndicator extends StatelessWidget {
  final bool isEncrypted;

  const ChatEncryptionIndicator({super.key, required this.isEncrypted});

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: isEncrypted ? 'End-to-end encrypted' : 'Not encrypted',
      child: Icon(
        isEncrypted ? Icons.lock : Icons.lock_open,
        size: 16,
        color: isEncrypted ? Colors.green[400] : Colors.red[400],
      ),
    );
  }
}
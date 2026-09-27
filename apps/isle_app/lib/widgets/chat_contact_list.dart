import 'package:flutter/material.dart';
import 'package:models/models.dart' show Conversation;

/// ChatContactList manages a scrollable list of conversations with search and unread counts.
class ChatContactList extends StatelessWidget {
  final List<Conversation> conversations;
  final String selectedId;
  final String searchQuery;
  final String bridgeStatus;
  final Map<String, int> unreadCounts;
  final ValueChanged<String> onSelect;

  const ChatContactList({
    super.key,
    required this.conversations,
    required this.selectedId,
    required this.searchQuery,
    required this.bridgeStatus,
    required this.unreadCounts,
    required this.onSelect,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final filtered = conversations.where((c) {
      if (searchQuery.isEmpty) return true;
      final query = searchQuery.toLowerCase();
      return c.name.toLowerCase().contains(query) ||
             c.participants.any((p) => p.toLowerCase().contains(query));
    }).toList();

    return Column(
      children: [
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          child: Row(
            children: [
              Text(
                'Conversations (${conversations.length})',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold, color: theme.colorScheme.onSurfaceVariant),
              ),
              const Spacer(),
              _BridgeStatusBadge(status: bridgeStatus),
            ],
          ),
        ),
        Expanded(
          child: filtered.isEmpty
              ? Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(Icons.chat_bubble_outline, size: 64, color: Colors.grey[700]),
                      const SizedBox(height: 12),
                      Text('No conversations yet', style: TextStyle(fontSize: 18, color: Colors.grey[500])),
                      const SizedBox(height: 4),
                      Text('Tap + to start a new conversation', style: TextStyle(fontSize: 16, color: Colors.grey[600])),
                    ],
                  ),
                )
              : ListView.builder(
                  itemCount: filtered.length,
                  itemBuilder: (ctx, i) {
                    final c = filtered[i];
                    final isSelected = c.id == selectedId;
                    final unread = unreadCounts[c.id] ?? 0;
                    return ListTile(
                      dense: true,
                      selected: isSelected,
                      selectedTileColor: theme.colorScheme.primaryContainer.withOpacity(0.3),
                      leading: Stack(
                        children: [
                          CircleAvatar(
                            radius: 20,
                            backgroundColor: theme.colorScheme.primaryContainer,
                            child: Text(
                              c.name.isNotEmpty ? c.name[0].toUpperCase() : '?',
                              style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold, color: theme.colorScheme.onPrimaryContainer),
                            ),
                          ),
                          if (unread > 0)
                            Positioned(
                              right: 0,
                              top: 0,
                              child: Container(
                                padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
                                decoration: BoxDecoration(color: Colors.red, borderRadius: BorderRadius.circular(10)),
                                child: Text('$unread', style: const TextStyle(fontSize: 10, color: Colors.white, fontWeight: FontWeight.bold)),
                              ),
                            ),
                        ],
                      ),
                      title: Text(c.name, style: const TextStyle(fontWeight: FontWeight.w600)),
                      subtitle: Text(
                        c.description.isNotEmpty ? c.description : '${c.participants.length} participants',
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(fontSize: 12, color: theme.colorScheme.onSurfaceVariant),
                      ),
                      trailing: Text('#${c.id.substring(0, 8)}', style: TextStyle(fontSize: 12, color: Colors.grey[600])),
                      onTap: () => onSelect(c.id),
                    );
                  },
                ),
        ),
      ],
    );
  }
}

class _BridgeStatusBadge extends StatelessWidget {
  final String status;
  const _BridgeStatusBadge({required this.status});

  @override
  Widget build(BuildContext context) {
    final connected = status == 'connected';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: connected ? Colors.green.withOpacity(0.1) : Colors.orange.withOpacity(0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: connected ? Colors.green.withOpacity(0.3) : Colors.orange.withOpacity(0.3)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(connected ? Icons.wifi : Icons.wifi_off, size: 12, color: connected ? Colors.green : Colors.orange),
          const SizedBox(width: 4),
          Text(
            connected ? 'Connected' : 'Disconnected',
            style: TextStyle(fontSize: 11, fontWeight: FontWeight.w600, color: connected ? Colors.green : Colors.orange),
          ),
        ],
      ),
    );
  }
}
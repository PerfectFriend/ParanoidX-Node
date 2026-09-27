import 'package:flutter/material.dart';

/// StatusBadge manages statusbadge functionality.
class StatusBadge extends StatelessWidget {
  final String? label;
  final String? status;
  final bool? active;
  final double size;

  const StatusBadge({super.key, this.label, this.status, this.active, this.size = 12});

  @override
  Widget build(BuildContext context) {
    final displayText = label ?? status ?? 'unknown';
    final color = active != null
        ? (active! ? Colors.green : Colors.red)
        : switch (status) {
            'ok' || 'online' || 'active' || 'healthy' => Colors.green,
            'warn' || 'pending' => Colors.orange,
            'fail' || 'offline' || 'cancelled' => Colors.red,
            _ => Colors.grey,
          };
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: size,
          height: size,
          decoration: BoxDecoration(
            color: color,
            shape: BoxShape.circle,
          ),
        ),
        const SizedBox(width: 4),
        Text(displayText, style: TextStyle(color: color, fontSize: size)),
      ],
    );
  }
}

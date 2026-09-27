import 'package:flutter/material.dart';

/// SectionCard manages sectioncard functionality.
class SectionCard extends StatelessWidget {
  final String title;
  final IconData icon;
  final List<Widget> children;
  final Widget? trailing;
  final Widget? child;

  const SectionCard({
    super.key,
    required this.title,
    required this.icon,
    this.children = const [],
    this.trailing,
    this.child,
  });

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 8),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(icon),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(title, style: Theme.of(context).textTheme.titleMedium),
                ),
                if (trailing != null) trailing!,
              ],
            ),
            const Divider(),
            if (child != null) child!,
            ...children,
          ],
        ),
      ),
    );
  }
}

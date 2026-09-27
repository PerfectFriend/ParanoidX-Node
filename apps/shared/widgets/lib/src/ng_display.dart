import 'package:flutter/material.dart';

/// NgDisplay manages ngdisplay functionality.
class NgDisplay extends StatelessWidget {
  final int ngAmount;
  final bool showTlr;
  final TextStyle? style;
  final String? label;

  const NgDisplay({
    super.key,
    required this.ngAmount,
    this.showTlr = true,
    this.style,
    this.label,
  });

  @override
  Widget build(BuildContext context) {
    final tlr = ngAmount / 31103480000;
    final ts = style ?? Theme.of(context).textTheme.titleMedium;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (label != null)
          Text(label!, style: Theme.of(context).textTheme.labelSmall?.copyWith(color: Colors.grey)),
        Text('$ngAmount ng', style: ts),
        if (showTlr)
          Text(
            '${tlr.toStringAsFixed(4)} TLR',
            style: Theme.of(context).textTheme.bodySmall?.copyWith(color: Colors.grey),
          ),
      ],
    );
  }
}

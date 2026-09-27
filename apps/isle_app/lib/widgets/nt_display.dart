import 'package:flutter/material.dart';

/// Nanotaler display — shows NT (base unit), TLR (Taler), or compact.
/// NtDisplay manages a widget that displays Nanotaler amounts in NT and TLR units.
class NtDisplay extends StatelessWidget {
  final int amountNt;
  final bool showTlr;
  final TextStyle? style;

  const NtDisplay({super.key, required this.amountNt, this.showTlr = true, this.style});

  static const int ntPerTlr = 31103480000;

  @override
  Widget build(BuildContext context) {
    final theme = style ?? Theme.of(context).textTheme.bodyMedium;
    final cs = Theme.of(context).colorScheme;
    final muted = cs.onSurfaceVariant;
    final tlr = amountNt / ntPerTlr;
    final nt = amountNt;

    if (showTlr && tlr >= 1) {
      return Text.rich(
        TextSpan(
          children: [
            TextSpan(
              text: tlr.toStringAsFixed(4),
              style: theme?.copyWith(fontWeight: FontWeight.bold),
            ),
            TextSpan(text: ' TLR', style: theme?.copyWith(fontSize: (theme?.fontSize ?? 14) - 2)),
            TextSpan(
              text: '  ($nt nt)',
              style: theme?.copyWith(fontSize: (theme?.fontSize ?? 14) - 3, color: muted),
            ),
          ],
        ),
      );
    }
    if (tlr >= 0.001) {
      return Text.rich(
        TextSpan(
          children: [
            TextSpan(text: '${nt.toStringAsFixed(0)}', style: theme?.copyWith(fontWeight: FontWeight.bold)),
            TextSpan(text: ' nt', style: theme?.copyWith(fontSize: (theme?.fontSize ?? 14) - 2)),
            TextSpan(
              text: '  (${tlr.toStringAsFixed(6)} TLR)',
              style: theme?.copyWith(fontSize: (theme?.fontSize ?? 14) - 3, color: muted),
            ),
          ],
        ),
      );
    }
    return Text.rich(
      TextSpan(
        children: [
          TextSpan(text: '${nt.toStringAsFixed(0)}', style: theme?.copyWith(fontWeight: FontWeight.bold)),
          TextSpan(text: ' nt', style: theme?.copyWith(fontSize: (theme?.fontSize ?? 14) - 2)),
        ],
      ),
    );
  }
}

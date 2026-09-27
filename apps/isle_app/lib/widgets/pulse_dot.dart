import 'package:flutter/material.dart';

/// PulseDot manages an animated pulsing dot used as a connection status indicator.
class PulseDot extends StatefulWidget {
  final double size;
  final Color color;
  final bool pulsing;
  const PulseDot({super.key, required this.size, required this.color, this.pulsing = true});

  @override
  State<PulseDot> createState() => _PulseDotState();
}

class _PulseDotState extends State<PulseDot> with SingleTickerProviderStateMixin {
  late AnimationController _ctrl;
  late Animation<double> _anim;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(vsync: this, duration: const Duration(milliseconds: 1200));
    _anim = Tween<double>(begin: 0.3, end: 1.0).animate(CurvedAnimation(parent: _ctrl, curve: Curves.easeInOut));
    if (widget.pulsing) _ctrl.repeat(reverse: true);
  }

  @override
  void didUpdateWidget(PulseDot old) {
    super.didUpdateWidget(old);
    if (widget.pulsing != old.pulsing) {
      if (widget.pulsing) _ctrl.repeat(reverse: true);
      else _ctrl.stop();
    }
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.pulsing) {
      return Container(width: widget.size, height: widget.size, decoration: BoxDecoration(color: widget.color, shape: BoxShape.circle));
    }
    return AnimatedBuilder(
      animation: _anim,
      builder: (_, __) => Container(
        width: widget.size, height: widget.size,
        decoration: BoxDecoration(color: widget.color.withValues(alpha: _anim.value), shape: BoxShape.circle),
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:royal_app/l10n/generated/app_localizations.dart';
import '../theme.dart';

/// Boot/Splash screen with island coat of arms, system language detection,
/// and transition to network bridge setup
class BootScreen extends StatefulWidget {
  final VoidCallback onBootComplete;

  const BootScreen({
    super.key,
    required this.onBootComplete,
  });

  @override
  State<BootScreen> createState() => _BootScreenState();
}

class _BootScreenState extends State<BootScreen> with TickerProviderStateMixin {
  late AnimationController _logoController;
  late AnimationController _textController;
  late AnimationController _progressController;
  late Animation<double> _logoScale;
  late Animation<double> _logoRotation;
  late Animation<double> _textOpacity;
  String _currentMessage = '';
  double _progress = 0.0;
  bool _languageDetected = false;
  Locale? _detectedLocale;
  late AppLocalizations _l10n;

  @override
  void initState() {
    super.initState();
    _initializeAnimations();
    _startBootSequence();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _l10n = AppLocalizations.of(context)!;
    if (_currentMessage.isEmpty) {
      _currentMessage = _l10n.bootMessage;
    }
  }

  void _initializeAnimations() {
    _logoController = AnimationController(
      duration: const Duration(milliseconds: 2000),
      vsync: this,
    );
    _textController = AnimationController(
      duration: const Duration(milliseconds: 1000),
      vsync: this,
    );
    _progressController = AnimationController(
      duration: const Duration(milliseconds: 3000),
      vsync: this,
    );

    _logoScale = Tween<double>(begin: 0.0, end: 1.0).animate(
      CurvedAnimation(parent: _logoController, curve: Curves.elasticOut),
    );
    _logoRotation = Tween<double>(begin: -0.1, end: 0.0).animate(
      CurvedAnimation(parent: _logoController, curve: Curves.easeOutBack),
    );
    _textOpacity = Tween<double>(begin: 0.0, end: 1.0).animate(
      CurvedAnimation(parent: _textController, curve: Curves.easeIn),
    );

    _logoController.forward();
    _textController.forward();
  }

  Future<void> _startBootSequence() async {
    // Step 1: Detect system language
    await _detectSystemLanguage();

    // Step 2: Show boot message
    setState(() => _currentMessage = _l10n.bootMessage);
    await Future.delayed(const Duration(milliseconds: 1500));

    // Step 3: Network bridge setup message
    setState(() => _currentMessage = _l10n.networkBridgeSetup);
    _progressController.forward();

    // Simulate progress
    for (int i = 0; i <= 100; i += 5) {
      await Future.delayed(const Duration(milliseconds: 100));
      if (mounted) {
        setState(() => _progress = i / 100);
      }
    }

    await Future.delayed(const Duration(milliseconds: 500));

    // Navigate to network bridge setup
    if (mounted) {
      widget.onBootComplete();
    }
  }

  Future<void> _detectSystemLanguage() async {
    try {
      final systemLocale = WidgetsBinding.instance.platformDispatcher.locale;
      final languageCode = systemLocale.languageCode;

      // Supported locales
      const supportedLocales = ['en', 'ru'];

      Locale detectedLocale;
      if (supportedLocales.contains(languageCode)) {
        detectedLocale = Locale(languageCode);
      } else {
        detectedLocale = const Locale('en'); // Default to English
      }

      if (mounted) {
        setState(() {
          _detectedLocale = detectedLocale;
          _languageDetected = true;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _detectedLocale = const Locale('en');
          _languageDetected = true;
        });
      }
    }
  }

  @override
  void dispose() {
    _logoController.dispose();
    _textController.dispose();
    _progressController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      backgroundColor: RoyalTheme.deepNavy,
      body: Stack(
        children: [
          // Background gradient
          Container(
            decoration: const BoxDecoration(
              gradient: RadialGradient(
                center: Alignment.center,
                radius: 1.5,
                colors: [
                  RoyalTheme.deepNavy,
                  Color(0xFF05080C),
                ],
              ),
            ),
          ),
          // Subtle pattern
          CustomPaint(
            size: Size.infinite,
            painter: _BackgroundPatternPainter(),
          ),
          // Main content
          Center(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                // Island Coat of Arms
                AnimatedBuilder(
                  animation:
                      Listenable.merge([_logoController, _textController]),
                  builder: (context, child) {
                    return Transform.scale(
                      scale: _logoScale.value,
                      child: Transform.rotate(
                        angle: _logoRotation.value * 0.1,
                        child: _buildCoatOfArms(),
                      ),
                    );
                  },
                ).animate().fadeIn(duration: 800.ms).scale(),

                const SizedBox(height: 48),

                // Boot message
                FadeTransition(
                  opacity: _textOpacity,
                  child: Column(
                    children: [
                      Text(
                        _currentMessage,
                        style: theme.textTheme.headlineLarge?.copyWith(
                          color: RoyalTheme.gold,
                          fontWeight: FontWeight.w700,
                          letterSpacing: 4,
                        ),
                      ),
                      const SizedBox(height: 8),
                      if (_languageDetected)
                        Text(
                          _detectedLocale?.languageCode == 'ru'
                              ? _l10n.russianDetected
                              : _l10n.englishDetected,
                          style: theme.textTheme.bodyMedium?.copyWith(
                            color: RoyalTheme.silver.withAlpha(180),
                          ),
                        ),
                    ],
                  ),
                ),

                const SizedBox(height: 64),

                // Progress bar
                FadeTransition(
                  opacity: _textOpacity,
                  child: SizedBox(
                    width: 300,
                    child: Column(
                      children: [
                        ClipRRect(
                          borderRadius: BorderRadius.circular(4),
                          child: LinearProgressIndicator(
                            value: _progress,
                            minHeight: 6,
                            backgroundColor: RoyalTheme.darkCard,
                            valueColor:
                                const AlwaysStoppedAnimation<Color>(RoyalTheme.gold),
                          ),
                        ),
                        const SizedBox(height: 12),
                        Text(
                          '${(_progress * 100).toInt()}%',
                          style: theme.textTheme.bodyMedium?.copyWith(
                            color: RoyalTheme.silver,
                            fontFeatures: const [FontFeature.tabularFigures()],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildCoatOfArms() {
    return Container(
      width: 180,
      height: 180,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        gradient: RadialGradient(
          colors: [
            RoyalTheme.gold.withAlpha(40),
            RoyalTheme.deepNavy,
          ],
        ),
        border: Border.all(
          color: RoyalTheme.gold.withAlpha(120),
          width: 2,
        ),
        boxShadow: [
          BoxShadow(
            color: RoyalTheme.gold.withAlpha(30),
            blurRadius: 40,
            spreadRadius: 5,
          ),
        ],
      ),
      child: Center(
        child: Stack(
          alignment: Alignment.center,
          children: [
            // Outer ring
            Container(
              width: 150,
              height: 150,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(
                  color: RoyalTheme.gold.withAlpha(80),
                  width: 1.5,
                ),
              ),
            ),
            // Inner ring
            Container(
              width: 110,
              height: 110,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(
                  color: RoyalTheme.gold.withAlpha(60),
                  width: 1,
                ),
              ),
            ),
            // Central symbol - Crown
            const Icon(
              Icons.diamond_rounded,
              size: 64,
              color: RoyalTheme.gold,
            )
                .animate(
                    onPlay: (controller) => controller.repeat(reverse: true))
                .shimmer(duration: 2000.ms, color: RoyalTheme.silver),
            // Cross behind crown
            Icon(
              Icons.add_rounded,
              size: 48,
              color: RoyalTheme.silver.withAlpha(100),
            ),
          ],
        ),
      ),
    );
  }
}

class _BackgroundPatternPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = RoyalTheme.gold.withAlpha(5)
      ..strokeWidth = 0.5;

    const spacing = 60.0;
    for (double x = 0; x < size.width; x += spacing) {
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), paint);
    }
    for (double y = 0; y < size.height; y += spacing) {
      canvas.drawLine(Offset(0, y), Offset(size.width, y), paint);
    }

    // Corner accents
    final accentPaint = Paint()
      ..color = RoyalTheme.gold.withAlpha(15)
      ..strokeWidth = 2
      ..style = PaintingStyle.stroke;

    const accentSize = 80.0;
    const accentInset = 40.0;

    // Top-left
    canvas.drawPath(
      Path()
        ..moveTo(accentInset, accentInset + accentSize)
        ..lineTo(accentInset, accentInset)
        ..lineTo(accentInset + accentSize, accentInset),
      accentPaint,
    );
    // Top-right
    canvas.drawPath(
      Path()
        ..moveTo(size.width - accentInset - accentSize, accentInset)
        ..lineTo(size.width - accentInset, accentInset)
        ..lineTo(size.width - accentInset, accentInset + accentSize),
      accentPaint,
    );
    // Bottom-left
    canvas.drawPath(
      Path()
        ..moveTo(accentInset, size.height - accentInset - accentSize)
        ..lineTo(accentInset, size.height - accentInset)
        ..lineTo(accentInset + accentSize, size.height - accentInset),
      accentPaint,
    );
    // Bottom-right
    canvas.drawPath(
      Path()
        ..moveTo(
            size.width - accentInset - accentSize, size.height - accentInset)
        ..lineTo(size.width - accentInset, size.height - accentInset)
        ..lineTo(
            size.width - accentInset, size.height - accentInset - accentSize),
      accentPaint,
    );
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

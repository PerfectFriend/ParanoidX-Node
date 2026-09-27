import 'package:flutter/material.dart';

/// RoyalTheme manages royaltheme functionality.
class RoyalTheme {
  static const Color gold = Color(0xFFFFD700);
  static const Color silver = Color(0xFFC0C0C0);
  static const Color deepNavy = Color(0xFF0D1117);
  static const Color darkCard = Color(0xFF161B22);
  static const Color darkScaffold = Color(0xFF0D1117);
  static const Color darkSurface = Color(0xFF0D1117);
  static const Color accent = Color(0xFF58A6FF);
  static const Color green = Color(0xFF3FB950);
  static const Color red = Color(0xFFF85149);
  static const Color orange = Color(0xFFD29922);
  static const Color purple = Color(0xFFBC8CFF);
  static const Color teal = Color(0xFF56D4DD);

  static ThemeData get dark {
    return ThemeData(
      useMaterial3: true,
      brightness: Brightness.dark,
      scaffoldBackgroundColor: deepNavy,
      colorScheme: const ColorScheme.dark(
        primary: gold,
        secondary: silver,
        surface: darkCard,
        error: red,
        tertiary: accent,
      ),
      appBarTheme: const AppBarTheme(
        backgroundColor: darkCard,
        foregroundColor: Colors.white,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: TextStyle(
          color: Colors.white,
          fontSize: 22,
          fontWeight: FontWeight.w600,
        ),
      ),
      cardTheme: CardThemeData(
        color: darkCard,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(20),
          side: const BorderSide(color: Color(0xFF30363D), width: 1),
        ),
      ),
      navigationRailTheme: NavigationRailThemeData(
        backgroundColor: darkCard,
        indicatorColor: gold.withAlpha(40),
        selectedLabelTextStyle: const TextStyle(
          color: gold,
          fontWeight: FontWeight.w600,
          fontSize: 14,
        ),
        unselectedLabelTextStyle: const TextStyle(
          color: Color(0xFF8B949E),
          fontSize: 13,
        ),
        labelType: NavigationRailLabelType.all,
        minExtendedWidth: 200,
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: const Color(0xFF21262D),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(14),
          borderSide: const BorderSide(color: Color(0xFF30363D)),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(14),
          borderSide: const BorderSide(color: Color(0xFF30363D)),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(14),
          borderSide: const BorderSide(color: gold, width: 2),
        ),
        labelStyle: const TextStyle(color: Color(0xFF8B949E), fontSize: 15),
        hintStyle: const TextStyle(color: Color(0xFF484F58), fontSize: 14),
        contentPadding: const EdgeInsets.symmetric(horizontal: 20, vertical: 18),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: gold,
          foregroundColor: deepNavy,
          padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 18),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(14),
          ),
          textStyle: const TextStyle(
            fontWeight: FontWeight.w600,
            fontSize: 16,
          ),
        ),
      ),
      textTheme: TextTheme(
        headlineLarge: const TextStyle(
          fontSize: 32, fontWeight: FontWeight.w700, color: Colors.white,
        ),
        headlineMedium: const TextStyle(
          fontSize: 26, fontWeight: FontWeight.w600, color: Colors.white,
        ),
        titleLarge: const TextStyle(
          fontSize: 20, fontWeight: FontWeight.w600, color: Colors.white,
        ),
        titleMedium: const TextStyle(
          fontSize: 18, fontWeight: FontWeight.w500, color: Colors.white,
        ),
        bodyLarge: const TextStyle(
          fontSize: 16, color: Color(0xFFC9D1D9),
        ),
        bodyMedium: const TextStyle(
          fontSize: 15, color: Color(0xFF8B949E),
        ),
        labelLarge: const TextStyle(
          fontSize: 15, fontWeight: FontWeight.w600,
          color: Color(0xFFC9D1D9),
        ),
      ),
      dividerTheme: const DividerThemeData(
        color: Color(0xFF21262D),
        thickness: 1,
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: darkCard,
        contentTextStyle: const TextStyle(color: Colors.white),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  static BoxDecoration glassCard({double opacity = 0.05}) {
    return BoxDecoration(
      color: Colors.white.withAlpha((opacity * 255).round()),
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: Colors.white.withAlpha(25)),
    );
  }

  static BoxDecoration gradientCard({List<Color>? colors}) {
    return BoxDecoration(
      gradient: LinearGradient(
        colors: colors ?? [darkCard, const Color(0xFF1C2128)],
        begin: Alignment.topLeft,
        end: Alignment.bottomRight,
      ),
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: const Color(0xFF30363D)),
    );
  }

  static BoxDecoration glowBorder({Color color = gold}) {
    return BoxDecoration(
      color: darkCard,
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: color.withAlpha(80), width: 1.5),
      boxShadow: [
        BoxShadow(
          color: color.withAlpha(25),
          blurRadius: 12,
          spreadRadius: 1,
        ),
      ],
    );
  }

  static Shader get goldShader => const LinearGradient(
    colors: [Color(0xFFFFD700), Color(0xFFFFA500)],
  ).createShader(const Rect.fromLTWH(0, 0, 200, 50));
}

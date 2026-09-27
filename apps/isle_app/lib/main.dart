import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity;

import '../widgets/bottom_nav_bar.dart';
import '../screens/dashboard_screen.dart';
import '../screens/wallet_screen.dart';
import '../screens/market_screen.dart';
import '../screens/vault_screen.dart';
import '../screens/radio_screen.dart';
import '../screens/simplex_chat_screen.dart';
import '../screens/welcome_screen.dart';

void main() {
  runApp(const IsleApp());
}

class IsleApp extends StatelessWidget {
  const IsleApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'The Isle',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1A237E),
          brightness: Brightness.light,
        ),
        useMaterial3: true,
        fontFamily: 'Roboto',
      ),
      darkTheme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1A237E),
          brightness: Brightness.dark,
        ),
        useMaterial3: true,
        fontFamily: 'Roboto',
      ),
      themeMode: ThemeMode.system,
      home: const WelcomeScreen(initialLocale: 'en'),
      onGenerateRoute: (settings) {
        if (settings.name == '/main') {
          final identity = settings.arguments as Identity;
          final client = SimplexApiClient(baseUrl: 'http://127.0.0.1:8080');
          return MaterialPageRoute(
            builder: (_) => MainScreen(identity: identity, client: client),
          );
        }
        return null;
      },
      debugShowCheckedModeBanner: false,
    );
  }
}

class MainScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;
  const MainScreen({super.key, required this.client, required this.identity});

  @override
  State<MainScreen> createState() => _MainScreenState();
}

class _MainScreenState extends State<MainScreen> {
  int _currentIndex = 0;

  late final List<Widget> _screens;

  @override
  void initState() {
    super.initState();
    _screens = [
      DashboardScreen(client: widget.client, identity: widget.identity),
      SimplexChatScreen(client: widget.client, identity: widget.identity),
      WalletScreen(client: widget.client, identity: widget.identity),
      RadioScreen(client: widget.client, identity: widget.identity),
      MarketScreen(client: widget.client, identity: widget.identity),
      VaultScreen(client: widget.client, identity: widget.identity),
    ];
  }

  void _onTabTapped(int index) {
    setState(() => _currentIndex = index);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: IndexedStack(index: _currentIndex, children: _screens),
      bottomNavigationBar: BottomNavBar(
        currentIndex: _currentIndex,
        onTap: _onTabTapped,
      ),
    );
  }
}

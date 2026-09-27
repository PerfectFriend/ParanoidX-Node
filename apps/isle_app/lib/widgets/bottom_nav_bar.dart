import 'package:flutter/material.dart';

/// BottomNavBar — навигация для гражданского приложения
class BottomNavBar extends StatelessWidget {
  final int currentIndex;
  final ValueChanged<int> onTap;

  const BottomNavBar({super.key, required this.currentIndex, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return NavigationBar(
      selectedIndex: currentIndex,
      onDestinationSelected: onTap,
      height: 64,
      labelBehavior: NavigationDestinationLabelBehavior.onlyShowSelected,
      destinations: const [
        NavigationDestination(icon: Icon(Icons.dashboard), label: 'Dashboard'),
        NavigationDestination(icon: Icon(Icons.chat), label: 'Chat'),
        NavigationDestination(icon: Icon(Icons.wallet), label: 'Wallet'),
        NavigationDestination(icon: Icon(Icons.radio), label: 'Radio'),
        NavigationDestination(icon: Icon(Icons.storefront), label: 'Market'),
        NavigationDestination(icon: Icon(Icons.folder), label: 'Vault'),
      ],
    );
  }
}

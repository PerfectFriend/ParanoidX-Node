import 'package:flutter/material.dart';
import '../services/radio_player.dart';
import '../services/tor_manager.dart';
import '../services/vpn_manager.dart';

/// Values for the topbarnodehealth domain.
enum TopBarNodeHealth { up, down, reconnecting }

/// TopBar manages the persistent top bar with health indicators, radio controls, and system actions.
class TopBar extends StatelessWidget {
  final RadioPlayer player;
  final TorManager? torMgr;
  final VpnManager? vpnMgr;
  final bool healthSmp;
  final bool healthXftp;
  final bool healthRadio;
  final bool healthVpn;
  final TopBarNodeHealth nodeHealth;
  final VoidCallback? onLock;
  final VoidCallback? onSwitchAccount;
  final VoidCallback? onOpenSettings;
  final String buildVersion;

  const TopBar({
    super.key,
    required this.player,
    this.torMgr,
    this.vpnMgr,
    this.healthSmp = false,
    this.healthXftp = false,
    this.healthRadio = false,
    this.healthVpn = false,
    this.nodeHealth = TopBarNodeHealth.down,
    this.onLock,
    this.onSwitchAccount,
    this.onOpenSettings,
    this.buildVersion = '',
  });

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: player,
      builder: (context, _) {
        final hasError = player.error != null && !player.playing;
        return Container(
          height: 48,
          decoration: BoxDecoration(
            color: Colors.grey.shade900,
            border: Border(bottom: BorderSide(color: Colors.grey.shade700)),
          ),
          padding: const EdgeInsets.symmetric(horizontal: 10),
          child: Row(
            children: [
              // Left: health indicators
              if (vpnMgr != null)
                ValueListenableBuilder<VpnInfo>(
                  valueListenable: vpnMgr!,
                  builder: (_, vp, __) {
                    final ok = vp.status == VpnStatus.connected;
                    final cfg = vp.status == VpnStatus.configuring;
                    return Padding(
                      padding: const EdgeInsets.only(right: 6),
                      child: _healthDot('VPN', ok, Colors.blue, cfg ? Colors.orange : Colors.grey),
                    );
                  },
                ),
              if (torMgr != null) _healthDot('TOR', torMgr!.isRunning, Colors.green, Colors.red),
              const SizedBox(width: 6),
              _healthDot('SMP', healthSmp, Colors.green, Colors.grey),
              const SizedBox(width: 6),
              _healthDot('XFTP', healthXftp, Colors.green, Colors.grey),
              const SizedBox(width: 6),
              _healthDot('RADIO', healthRadio, Colors.green, Colors.grey),
              const SizedBox(width: 6),
              _nodeHealthDot(),
              const SizedBox(width: 10),
              Container(width: 1, height: 28, color: Colors.grey.shade700),
              const SizedBox(width: 10),

              // Center: track title or error or offline
              Expanded(
                child: hasError
                    ? Text(player.error!, style: const TextStyle(fontSize: 11, color: Colors.redAccent),
                        overflow: TextOverflow.ellipsis, maxLines: 1)
                    : player.playing
                        ? Text(player.currentTrackTitle.isNotEmpty ? player.currentTrackTitle : 'Streaming...',
                            style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w500, color: Colors.white),
                            textAlign: TextAlign.center,
                            overflow: TextOverflow.ellipsis, maxLines: 1)
                        : Text(player.selectedStation != null ? player.currentTrackTitle : 'Radio',
                            style: TextStyle(fontSize: 13, fontWeight: FontWeight.w400, color: Colors.grey[500]),
                            textAlign: TextAlign.center,
                            overflow: TextOverflow.ellipsis, maxLines: 1),
              ),
              const SizedBox(width: 10),
              Container(width: 1, height: 28, color: Colors.grey.shade700),
              const SizedBox(width: 8),

              // Right: controls
              if (buildVersion.isNotEmpty)
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: Colors.grey.shade800,
                    borderRadius: BorderRadius.circular(4),
                    border: Border.all(color: Colors.grey.shade600, width: 0.5),
                  ),
                  child: Text('v$buildVersion', style: const TextStyle(fontSize: 10, color: Colors.grey)),
                ),
              const SizedBox(width: 4),
              if (player.playlist.length > 1)
                _btn(Icons.skip_previous, 'Previous',
                    player.currentTrackIndex > 0 ? player.skipPrevious : null),
              GestureDetector(
                onTap: player.togglePlayPause,
                child: Container(
                  width: 34,
                  height: 34,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: player.playing ? Colors.red.shade700 : Colors.green.shade700,
                    boxShadow: [
                      BoxShadow(
                        color: (player.playing ? Colors.red : Colors.green).withValues(alpha: 0.4),
                        blurRadius: 6, spreadRadius: 1,
                      ),
                    ],
                  ),
                  child: Icon(
                    player.playing ? Icons.stop : Icons.play_arrow,
                    size: 22, color: Colors.white,
                  ),
                ),
              ),
              if (player.playlist.length > 1)
                _btn(Icons.skip_next, 'Next',
                    player.currentTrackIndex + 1 < player.playlist.length ? player.skipNext : null),
              const SizedBox(width: 4),
              _VolumeSlider(player: player),
              const SizedBox(width: 2),
              _btn(Icons.swap_horiz, 'Switch Account', onSwitchAccount),
              if (onLock != null) _btn(Icons.lock_outline, 'Lock', onLock),
              if (onOpenSettings != null) _btn(Icons.settings, 'Settings', onOpenSettings),
            ],
          ),
        );
      },
    );
  }

  Widget _nodeHealthDot() {
    IconData icon;
    Color color;
    String label;
    switch (nodeHealth) {
      case TopBarNodeHealth.up:
        icon = Icons.check_circle;
        color = Colors.green;
        label = 'NODE';
      case TopBarNodeHealth.down:
        icon = Icons.error;
        color = Colors.red;
        label = 'DOWN';
      case TopBarNodeHealth.reconnecting:
        icon = Icons.sync;
        color = Colors.orange;
        label = 'SYNC';
    }
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 12, color: color),
        const SizedBox(width: 3),
        Text(label, style: TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: color)),
      ],
    );
  }

  Widget _healthDot(String label, bool ok, Color onColor, Color offColor) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(width: 10, height: 10,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: ok ? onColor : offColor,
            border: ok ? Border.all(color: onColor.withValues(alpha: 0.5), width: 1) : null,
          ),
        ),
        const SizedBox(width: 3),
        Text(label, style: TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: ok ? onColor : offColor)),
      ],
    );
  }

  Widget _btn(IconData icon, String tooltip, VoidCallback? onPressed) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 2),
      child: IconButton(
        icon: Icon(icon, size: 20, color: onPressed != null ? Colors.grey.shade300 : Colors.grey.shade700),
        onPressed: onPressed,
        padding: EdgeInsets.zero,
        constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
        splashRadius: 16,
        tooltip: tooltip,
      ),
    );
  }
}

class _VolumeSlider extends StatelessWidget {
  final RadioPlayer player;
  const _VolumeSlider({required this.player});

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 140,
      child: Row(
        children: [
          Icon(Icons.volume_down, size: 16, color: Colors.grey.shade400),
          Expanded(
            child: SliderTheme(
              data: SliderTheme.of(context).copyWith(
                activeTrackColor: Colors.greenAccent,
                inactiveTrackColor: Colors.grey.shade700,
                thumbColor: Colors.white,
                overlayColor: Colors.greenAccent.withValues(alpha: 0.15),
                trackHeight: 5,
                thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 8),
                overlayShape: const RoundSliderOverlayShape(overlayRadius: 16),
              ),
              child: Slider(
                value: player.volume,
                onChanged: player.setVolume,
                min: 0.0, max: 1.0, divisions: 20,
              ),
            ),
          ),
          Icon(Icons.volume_up, size: 16, color: Colors.grey.shade400),
          const SizedBox(width: 4),
          SizedBox(width: 32,
            child: Text('${(player.volume * 100).toInt()}%',
                style: const TextStyle(fontSize: 11, color: Colors.white70)),
          ),
        ],
      ),
    );
  }
}

/// HealthStatus manages healthstatus functionality.
class HealthStatus {
  final bool dockerRunning;
  final bool torRunning;
  final bool bridgeConnected;
  final int diskUsagePercent;
  final int diskAvailableMb;

  const HealthStatus({
    this.dockerRunning = false,
    this.torRunning = false,
    this.bridgeConnected = false,
    this.diskUsagePercent = 0,
    this.diskAvailableMb = 0,
  });

  factory HealthStatus.fromJson(Map<String, dynamic> json) {
    return HealthStatus(
      dockerRunning: json['docker_running'] ?? false,
      torRunning: json['tor_running'] ?? false,
      bridgeConnected: json['bridge_connected'] ?? false,
      diskUsagePercent: (json['disk_usage_percent'] ?? 0).toInt(),
      diskAvailableMb: (json['disk_available_mb'] ?? 0).toInt(),
    );
  }

/// Returns the current allOk value.
  bool get allOk => dockerRunning && torRunning && bridgeConnected;
}

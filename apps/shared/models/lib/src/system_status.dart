/// System status models for the Royal Admin interface.

/// NodeInfo represents basic node information from /api/admin/info
class NodeInfo {
  final String status;
  final int uptimeSeconds;
  final int totalBanknotes;
  final int totalHolders;
  final int reserveNg;
  final int supplyNg;
  final int activeUsers;

  const NodeInfo({
    required this.status,
    this.uptimeSeconds = 0,
    this.totalBanknotes = 0,
    this.totalHolders = 0,
    this.reserveNg = 0,
    this.supplyNg = 0,
    this.activeUsers = 0,
  });

  factory NodeInfo.fromJson(Map<String, dynamic> json) {
    return NodeInfo(
      status: json['status'] ?? 'unknown',
      uptimeSeconds: (json['uptime'] ?? 0).toInt(),
      totalBanknotes: (json['total_banknotes'] ?? 0).toInt(),
      totalHolders: (json['total_holders'] ?? 0).toInt(),
      reserveNg: (json['reserve_ng'] ?? 0).toInt(),
      supplyNg: (json['supply_ng'] ?? 0).toInt(),
      activeUsers: (json['active_users'] ?? 0).toInt(),
    );
  }

  String get uptimeFormatted {
    final h = uptimeSeconds ~/ 3600;
    final m = (uptimeSeconds % 3600) ~/ 60;
    final s = uptimeSeconds % 60;
    return '${h}h ${m}m ${s}s';
  }
}

/// SystemMetrics represents system metrics from /api/admin/metrics/system
class SystemMetrics {
  final int cpuUsagePercent;
  final int memoryUsagePercent;
  final int diskUsagePercent;
  final int diskAvailableMb;
  final int networkRxBytes;
  final int networkTxBytes;
  final int goroutines;
  final int heapAllocMb;

  const SystemMetrics({
    this.cpuUsagePercent = 0,
    this.memoryUsagePercent = 0,
    this.diskUsagePercent = 0,
    this.diskAvailableMb = 0,
    this.networkRxBytes = 0,
    this.networkTxBytes = 0,
    this.goroutines = 0,
    this.heapAllocMb = 0,
  });

  factory SystemMetrics.fromJson(Map<String, dynamic> json) {
    return SystemMetrics(
      cpuUsagePercent: (json['cpu_percent'] ?? 0).toInt(),
      memoryUsagePercent: (json['memory_percent'] ?? 0).toInt(),
      diskUsagePercent: (json['disk_percent'] ?? 0).toInt(),
      diskAvailableMb: (json['disk_available_mb'] ?? 0).toInt(),
      networkRxBytes: (json['network_rx_bytes'] ?? 0).toInt(),
      networkTxBytes: (json['network_tx_bytes'] ?? 0).toInt(),
      goroutines: (json['goroutines'] ?? 0).toInt(),
      heapAllocMb: (json['heap_alloc_mb'] ?? 0).toInt(),
    );
  }
}

/// DockerStatus represents Docker status from /api/admin/docker
class DockerStatus {
  final bool running;
  final int containerCount;
  final int runningContainers;
  final int stoppedContainers;
  final List<ContainerStatus> containers;

  const DockerStatus({
    this.running = false,
    this.containerCount = 0,
    this.runningContainers = 0,
    this.stoppedContainers = 0,
    this.containers = const [],
  });

  factory DockerStatus.fromJson(Map<String, dynamic> json) {
    return DockerStatus(
      running: json['running'] ?? false,
      containerCount: (json['container_count'] ?? 0).toInt(),
      runningContainers: (json['running_containers'] ?? 0).toInt(),
      stoppedContainers: (json['stopped_containers'] ?? 0).toInt(),
      containers: (json['containers'] as List? ?? []).map((e) => ContainerStatus.fromJson(e)).toList(),
    );
  }
}

/// ContainerStatus represents a single Docker container status
class ContainerStatus {
  final String id;
  final String name;
  final String image;
  final String status;
  final int cpuPercent;
  final int memoryPercent;
  final int memoryUsageMb;
  final int memoryLimitMb;

  const ContainerStatus({
    this.id = '',
    this.name = '',
    this.image = '',
    this.status = 'unknown',
    this.cpuPercent = 0,
    this.memoryPercent = 0,
    this.memoryUsageMb = 0,
    this.memoryLimitMb = 0,
  });

  factory ContainerStatus.fromJson(Map<String, dynamic> json) {
    return ContainerStatus(
      id: json['id'] ?? '',
      name: json['name'] ?? '',
      image: json['image'] ?? '',
      status: json['status'] ?? 'unknown',
      cpuPercent: (json['cpu_percent'] ?? 0).toInt(),
      memoryPercent: (json['memory_percent'] ?? 0).toInt(),
      memoryUsageMb: (json['memory_usage_mb'] ?? 0).toInt(),
      memoryLimitMb: (json['memory_limit_mb'] ?? 0).toInt(),
    );
  }
}

/// ServiceStatus represents service status from /api/admin/service/status
class ServiceStatus {
  final bool tor;
  final bool bridge;
  final bool simplexNode;
  final bool postgres;
  final bool redis;
  final bool allOk;

  const ServiceStatus({
    this.tor = false,
    this.bridge = false,
    this.simplexNode = false,
    this.postgres = false,
    this.redis = false,
    this.allOk = false,
  });

  factory ServiceStatus.fromJson(Map<String, dynamic> json) {
    return ServiceStatus(
      tor: json['tor'] ?? false,
      bridge: json['bridge'] ?? false,
      simplexNode: json['simplex_node'] ?? false,
      postgres: json['postgres'] ?? false,
      redis: json['redis'] ?? false,
      allOk: json['all_ok'] ?? false,
    );
  }
}

/// ServiceActionResult represents the result of a service action (restart, etc.)
class ServiceActionResult {
  final bool success;
  final String message;
  const ServiceActionResult({this.success = false, this.message = ''});
  factory ServiceActionResult.fromJson(Map<String, dynamic> json) => ServiceActionResult(
    success: json['success'] ?? false,
    message: json['message'] ?? '',
  );
}

/// BackupResult represents backup operation result
class BackupResult {
  final bool success;
  final String path;
  final int sizeBytes;
  final String message;
  const BackupResult({this.success = false, this.path = '', this.sizeBytes = 0, this.message = ''});
  factory BackupResult.fromJson(Map<String, dynamic> json) => BackupResult(
    success: json['success'] ?? false,
    path: json['path'] ?? '',
    sizeBytes: (json['size_bytes'] ?? 0).toInt(),
    message: json['message'] ?? '',
  );
}

/// CleanupResult represents disk cleanup operation result
class CleanupResult {
  final bool success;
  final int freedBytes;
  final String message;
  const CleanupResult({this.success = false, this.freedBytes = 0, this.message = ''});
  factory CleanupResult.fromJson(Map<String, dynamic> json) => CleanupResult(
    success: json['success'] ?? false,
    freedBytes: (json['freed_bytes'] ?? 0).toInt(),
    message: json['message'] ?? '',
  );
}

/// Config represents system configuration
class Config {
  final Map<String, dynamic> data;
  const Config({this.data = const {}});
  factory Config.fromJson(Map<String, dynamic> json) => Config(data: json);
  Map<String, dynamic> toJson() => data;
}

/// MaintenanceMode represents maintenance mode status
class MaintenanceMode {
  final bool active;
  final String message;
  const MaintenanceMode({this.active = false, this.message = ''});
  factory MaintenanceMode.fromJson(Map<String, dynamic> json) => MaintenanceMode(
    active: json['active'] ?? false,
    message: json['message'] ?? '',
  );
}

/// EmergencyStop represents emergency stop status
class EmergencyStop {
  final bool enabled;
  final String triggeredAt;
  final String reason;
  const EmergencyStop({this.enabled = false, this.triggeredAt = '', this.reason = ''});
  factory EmergencyStop.fromJson(Map<String, dynamic> json) => EmergencyStop(
    enabled: json['enabled'] ?? false,
    triggeredAt: json['triggered_at'] ?? '',
    reason: json['reason'] ?? '',
  );
}

/// RateLimitStats represents rate limit statistics
class RateLimitStats {
  final int totalRequests;
  final int blockedRequests;
  final int activeClients;
  final double avgResponseMs;
  const RateLimitStats({this.totalRequests = 0, this.blockedRequests = 0, this.activeClients = 0, this.avgResponseMs = 0.0});
  factory RateLimitStats.fromJson(Map<String, dynamic> json) => RateLimitStats(
    totalRequests: (json['total_requests'] ?? 0).toInt(),
    blockedRequests: (json['blocked_requests'] ?? 0).toInt(),
    activeClients: (json['active_clients'] ?? 0).toInt(),
    avgResponseMs: (json['avg_response_ms'] ?? 0.0).toDouble(),
  );
}

/// ContainerActionResult represents the result of a container action (open/close)
class ContainerActionResult {
  final bool success;
  final String status;
  final String message;
  const ContainerActionResult({this.success = false, this.status = '', this.message = ''});
  factory ContainerActionResult.fromJson(Map<String, dynamic> json) => ContainerActionResult(
    success: json['success'] ?? false,
    status: json['status'] ?? '',
    message: json['message'] ?? '',
  );
}

/// Diagnostics represents system diagnostics from /api/admin/diagnostics
class Diagnostics {
  final int diskUsagePercent;
  final int diskAvailableMb;
  final int memoryUsagePercent;
  final int cpuUsagePercent;
  final int goroutines;
  final int heapAllocMb;
  final int heapSysMb;
  final String goVersion;

  const Diagnostics({
    this.diskUsagePercent = 0,
    this.diskAvailableMb = 0,
    this.memoryUsagePercent = 0,
    this.cpuUsagePercent = 0,
    this.goroutines = 0,
    this.heapAllocMb = 0,
    this.heapSysMb = 0,
    this.goVersion = '',
  });

  factory Diagnostics.fromJson(Map<String, dynamic> json) {
    return Diagnostics(
      diskUsagePercent: (json['disk_usage_percent'] ?? 0).toInt(),
      diskAvailableMb: (json['disk_available_mb'] ?? 0).toInt(),
      memoryUsagePercent: (json['memory_usage_percent'] ?? 0).toInt(),
      cpuUsagePercent: (json['cpu_usage_percent'] ?? 0).toInt(),
      goroutines: (json['goroutines'] ?? 0).toInt(),
      heapAllocMb: (json['heap_alloc_mb'] ?? 0).toInt(),
      heapSysMb: (json['heap_sys_mb'] ?? 0).toInt(),
      goVersion: json['go_version'] ?? '',
    );
  }
}
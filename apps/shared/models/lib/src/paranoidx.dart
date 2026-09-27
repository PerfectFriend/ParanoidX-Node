/// ParanoidX models for multi-layer proxy chain management
library models.src.paranoidx;

/// ParanoidX status response
class ParanoidXStatus {
  final bool overallHealthy;
  final List<LayerStatus> layers;
  final String? error;

  ParanoidXStatus({required this.overallHealthy, required this.layers, this.error});

  factory ParanoidXStatus.fromJson(Map<String, dynamic> json) {
    return ParanoidXStatus(
      overallHealthy: json['overall_healthy'] as bool? ?? false,
      layers: (json['layers'] as List<dynamic>?)
              ?.map((e) => LayerStatus.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
      error: json['error'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'overall_healthy': overallHealthy,
        'layers': layers.map((e) => e.toJson()).toList(),
        'error': error,
      };
}

/// Individual layer status
class LayerStatus {
  final String layer;
  final bool healthy;
  final String? message;
  final Map<String, dynamic>? details;

  LayerStatus({required this.layer, required this.healthy, this.message, this.details});

  factory LayerStatus.fromJson(Map<String, dynamic> json) {
    return LayerStatus(
      layer: json['layer'] as String? ?? '',
      healthy: json['healthy'] as bool? ?? false,
      message: json['message'] as String?,
      details: json['details'] as Map<String, dynamic>?,
    );
  }

  Map<String, dynamic> toJson() => {
        'layer': layer,
        'healthy': healthy,
        'message': message,
        'details': details,
      };
}

/// ParanoidX chain state
class ChainState {
  final String state; // up, down, building, tearing_down
  final String? message;
  final Map<String, dynamic>? details;

  ChainState({required this.state, this.message, this.details});

  factory ChainState.fromJson(Map<String, dynamic> json) {
    return ChainState(
      state: json['state'] as String? ?? 'down',
      message: json['message'] as String?,
      details: json['details'] as Map<String, dynamic>?,
    );
  }

  Map<String, dynamic> toJson() => {
        'state': state,
        'message': message,
        'details': details,
      };
}

/// ParanoidX configuration
class ParanoidXConfig {
  final bool v2rayEnabled;
  final bool vpnEnabled;
  final bool torEnabled;
  final Map<String, dynamic>? v2rayConfig;
  final Map<String, dynamic>? vpnConfig;
  final Map<String, dynamic>? torConfig;

  ParanoidXConfig({
    required this.v2rayEnabled,
    required this.vpnEnabled,
    required this.torEnabled,
    this.v2rayConfig,
    this.vpnConfig,
    this.torConfig,
  });

  factory ParanoidXConfig.fromJson(Map<String, dynamic> json) {
    return ParanoidXConfig(
      v2rayEnabled: json['v2ray_enabled'] as bool? ?? true,
      vpnEnabled: json['vpn_enabled'] as bool? ?? true,
      torEnabled: json['tor_enabled'] as bool? ?? true,
      v2rayConfig: json['v2ray_config'] as Map<String, dynamic>?,
      vpnConfig: json['vpn_config'] as Map<String, dynamic>?,
      torConfig: json['tor_config'] as Map<String, dynamic>?,
    );
  }

  Map<String, dynamic> toJson() => {
        'v2ray_enabled': v2rayEnabled,
        'vpn_enabled': vpnEnabled,
        'tor_enabled': torEnabled,
        'v2ray_config': v2rayConfig,
        'vpn_config': vpnConfig,
        'tor_config': torConfig,
      };
}

/// VPN Profile
class VpnProfile {
  final String name;
  final String? description;
  final String config;
  final bool active;
  final DateTime? createdAt;

  VpnProfile({required this.name, this.description, required this.config, this.active = false, this.createdAt});

  factory VpnProfile.fromJson(Map<String, dynamic> json) {
    return VpnProfile(
      name: json['name'] as String? ?? '',
      description: json['description'] as String?,
      config: json['config'] as String? ?? '',
      active: json['active'] as bool? ?? false,
      createdAt: json['created_at'] != null ? DateTime.parse(json['created_at'] as String) : null,
    );
  }

  Map<String, dynamic> toJson() => {
        'name': name,
        'description': description,
        'config': config,
        'active': active,
        'created_at': createdAt?.toIso8601String(),
      };
}

/// VPN Profiles list response
class VpnProfilesResponse {
  final List<VpnProfile> profiles;
  final String? active;

  VpnProfilesResponse({required this.profiles, this.active});

  factory VpnProfilesResponse.fromJson(Map<String, dynamic> json) {
    return VpnProfilesResponse(
      profiles: (json['profiles'] as List<dynamic>?)
              ?.map((e) => VpnProfile.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
      active: json['active'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'profiles': profiles.map((e) => e.toJson()).toList(),
        'active': active,
      };
}

/// Test results for chain connectivity
class ChainTestResult {
  final bool overall;
  final Map<String, bool> results;
  final String? message;
  final DateTime timestamp;

  ChainTestResult({required this.overall, required this.results, this.message, required this.timestamp});

  factory ChainTestResult.fromJson(Map<String, dynamic> json) {
    return ChainTestResult(
      overall: json['overall'] as bool? ?? false,
      results: (json['results'] as Map<String, dynamic>?)
              ?.map((k, v) => MapEntry(k, v as bool)) ??
          {},
      message: json['message'] as String?,
      timestamp: DateTime.parse(json['timestamp'] as String? ?? DateTime.now().toIso8601String()),
    );
  }

  Map<String, dynamic> toJson() => {
        'overall': overall,
        'results': results,
        'message': message,
        'timestamp': timestamp.toIso8601String(),
      };
}

/// Build chain result
class BuildChainResult {
  final bool success;
  final String? message;
  final ChainState? state;

  BuildChainResult({required this.success, this.message, this.state});

  factory BuildChainResult.fromJson(Map<String, dynamic> json) {
    return BuildChainResult(
      success: json['success'] as bool? ?? false,
      message: json['message'] as String?,
      state: json['state'] != null ? ChainState.fromJson(json['state'] as Map<String, dynamic>) : null,
    );
  }

  Map<String, dynamic> toJson() => {
        'success': success,
        'message': message,
        'state': state?.toJson(),
      };
}

/// Teardown chain result
class TeardownChainResult {
  final bool success;
  final String? message;

  TeardownChainResult({required this.success, this.message});

  factory TeardownChainResult.fromJson(Map<String, dynamic> json) {
    return TeardownChainResult(
      success: json['success'] as bool? ?? false,
      message: json['message'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'success': success,
        'message': message,
      };
}

/// Add VPN profile result
class AddVpnProfileResult {
  final bool success;
  final String? message;
  final VpnProfile? profile;

  AddVpnProfileResult({required this.success, this.message, this.profile});

  factory AddVpnProfileResult.fromJson(Map<String, dynamic> json) {
    return AddVpnProfileResult(
      success: json['success'] as bool? ?? false,
      message: json['message'] as String?,
      profile: json['profile'] != null ? VpnProfile.fromJson(json['profile'] as Map<String, dynamic>) : null,
    );
  }

  Map<String, dynamic> toJson() => {
        'success': success,
        'message': message,
        'profile': profile?.toJson(),
      };
}

/// VPN action result (up/down/delete)
class VpnActionResult {
  final bool success;
  final String? message;

  VpnActionResult({required this.success, this.message});

  factory VpnActionResult.fromJson(Map<String, dynamic> json) {
    return VpnActionResult(
      success: json['success'] as bool? ?? false,
      message: json['message'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'success': success,
        'message': message,
      };
}
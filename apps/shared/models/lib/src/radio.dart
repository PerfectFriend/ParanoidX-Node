/// RadioStation manages radiostation functionality.
library models.src.radio;

class RadioStation {
  final String id;
  final String name;
  final String type;
  final String lang;
  final String description;
  final bool enabled;
  final String? icon;

  RadioStation({
    required this.id,
    required this.name,
    required this.type,
    required this.lang,
    required this.description,
    required this.enabled,
    this.icon,
  });

  factory RadioStation.fromJson(Map<String, dynamic> json) {
    return RadioStation(
      id: json['id'] ?? '',
      name: json['name'] ?? '',
      type: json['type'] ?? 'mixed',
      lang: json['lang'] ?? 'en',
      description: json['description'] ?? '',
      enabled: json['enabled'] ?? false,
      icon: json['icon'],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'name': name,
      'type': type,
      'lang': lang,
      'description': description,
      'enabled': enabled,
      'icon': icon,
    };
  }

  String get languageLabel {
    switch (lang) {
      case 'en': return 'English';
      case 'ru': return 'Русский';
      case 'es': return 'Español';
      default: return lang;
    }
  }

  String get languageFlag {
    switch (lang) {
      case 'en': return '🇬🇧';
      case 'ru': return '🇷🇺';
      case 'es': return '🇪🇸';
      default: return '🌐';
    }
  }

  String get typeIcon {
    switch (type) {
      case 'music': return '🎵';
      case 'news': return '📰';
      case 'talk': return '🎙️';
      case 'mixed': return '📻';
      default: return '📻';
    }
  }
}

/// RadioTrack manages radiotrack functionality.
class RadioTrack {
  final String id;
  final String title;
  final int duration;
  final bool isAd;
  final bool isAnnounce;
  final String? streamUrl;

  RadioTrack({
    required this.id,
    required this.title,
    required this.duration,
    required this.isAd,
    required this.isAnnounce,
    this.streamUrl,
  });

  factory RadioTrack.fromJson(Map<String, dynamic> json) {
    return RadioTrack(
      id: json['id'] ?? '',
      title: json['title'] ?? '',
      duration: json['duration'] ?? 0,
      isAd: json['is_ad'] ?? false,
      isAnnounce: json['is_announce'] ?? false,
      streamUrl: json['stream_url'],
    );
  }

  String get typeLabel {
    if (isAd) return '📢 Реклама';
    if (isAnnounce) return '📢 Объявление';
    return '🎵 Трек';
  }
}

/// RadioSchedule represents the radio station schedule
class RadioSchedule {
  final List<RadioStationSchedule> stations;
  final int updatedAt;

  RadioSchedule({required this.stations, required this.updatedAt});

  factory RadioSchedule.fromJson(Map<String, dynamic> json) {
    return RadioSchedule(
      stations: (json['stations'] as List<dynamic>?)
              ?.map((e) => RadioStationSchedule.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
      updatedAt: json['updated_at'] as int? ?? 0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'stations': stations.map((e) => e.toJson()).toList(),
      'updated_at': updatedAt,
    };
  }
}

/// RadioStationSchedule represents a single station's schedule
class RadioStationSchedule {
  final String stationId;
  final String stationName;
  final List<ScheduledBlock> blocks;

  RadioStationSchedule({required this.stationId, required this.stationName, required this.blocks});

  factory RadioStationSchedule.fromJson(Map<String, dynamic> json) {
    return RadioStationSchedule(
      stationId: json['station_id'] as String? ?? '',
      stationName: json['station_name'] as String? ?? '',
      blocks: (json['blocks'] as List<dynamic>?)
              ?.map((e) => ScheduledBlock.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'station_id': stationId,
      'station_name': stationName,
      'blocks': blocks.map((e) => e.toJson()).toList(),
    };
  }
}

/// ScheduledBlock represents a time block in the schedule
class ScheduledBlock {
  final int startTime;
  final int endTime;
  final String title;
  final String type;
  final String? description;

  ScheduledBlock({required this.startTime, required this.endTime, required this.title, required this.type, this.description});

  factory ScheduledBlock.fromJson(Map<String, dynamic> json) {
    return ScheduledBlock(
      startTime: json['start_time'] as int? ?? 0,
      endTime: json['end_time'] as int? ?? 0,
      title: json['title'] as String? ?? '',
      type: json['type'] as String? ?? '',
      description: json['description'] as String?,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'start_time': startTime,
      'end_time': endTime,
      'title': title,
      'type': type,
      'description': description,
    };
  }
}

/// RadioScheduleContent represents the content of the radio schedule
class RadioScheduleContent {
  final Map<String, List<ScheduledBlock>> content;

  RadioScheduleContent({required this.content});

  factory RadioScheduleContent.fromJson(Map<String, dynamic> json) {
    final map = <String, List<ScheduledBlock>>{};
    json.forEach((key, value) {
      if (value is List) {
        map[key] = value.map((e) => ScheduledBlock.fromJson(e as Map<String, dynamic>)).toList();
      }
    });
    return RadioScheduleContent(content: map);
  }

  Map<String, dynamic> toJson() {
    return content.map((k, v) => MapEntry(k, v.map((e) => e.toJson()).toList()));
  }
}

/// AiContent represents AI-generated content for radio
class AiContent {
  final String id;
  final String title;
  final String content;
  final String type;
  final int createdAt;
  final int? updatedAt;

  AiContent({required this.id, required this.title, required this.content, required this.type, required this.createdAt, this.updatedAt});

  factory AiContent.fromJson(Map<String, dynamic> json) {
    return AiContent(
      id: json['id'] as String? ?? '',
      title: json['title'] as String? ?? '',
      content: json['content'] as String? ?? '',
      type: json['type'] as String? ?? '',
      createdAt: json['created_at'] as int? ?? 0,
      updatedAt: json['updated_at'] as int?,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'title': title,
      'content': content,
      'type': type,
      'created_at': createdAt,
      'updated_at': updatedAt,
    };
  }
}

/// AiResponse represents a response from the AI chat
class AiResponse {
  final String response;
  final String model;
  final int tokensUsed;
  final int createdAt;

  AiResponse({required this.response, required this.model, required this.tokensUsed, required this.createdAt});

  factory AiResponse.fromJson(Map<String, dynamic> json) {
    return AiResponse(
      response: json['response'] as String? ?? '',
      model: json['model'] as String? ?? '',
      tokensUsed: json['tokens_used'] as int? ?? 0,
      createdAt: json['created_at'] as int? ?? 0,
    );
  }
}

/// AiExplanation represents an AI explanation
class AiExplanation {
  final String explanation;
  final List<String> sources;
  final int confidence;

  AiExplanation({required this.explanation, required this.sources, required this.confidence});

  factory AiExplanation.fromJson(Map<String, dynamic> json) {
    return AiExplanation(
      explanation: json['explanation'] as String? ?? '',
      sources: (json['sources'] as List<dynamic>?)?.cast<String>() ?? [],
      confidence: json['confidence'] as int? ?? 0,
    );
  }
}

/// MemoryStats represents AI memory statistics
class MemoryStats {
  final int totalMemories;
  final int shortTermCount;
  final int longTermCount;
  final double usagePercent;

  MemoryStats({required this.totalMemories, required this.shortTermCount, required this.longTermCount, required this.usagePercent});

  factory MemoryStats.fromJson(Map<String, dynamic> json) {
    return MemoryStats(
      totalMemories: json['total_memories'] as int? ?? 0,
      shortTermCount: json['short_term_count'] as int? ?? 0,
      longTermCount: json['long_term_count'] as int? ?? 0,
      usagePercent: (json['usage_percent'] as num?)?.toDouble() ?? 0.0,
    );
  }
}

/// AiHealth represents AI service health
class AiHealth {
  final String status;
  final String model;
  final int uptime;
  final Map<String, dynamic> metrics;

  AiHealth({required this.status, required this.model, required this.uptime, required this.metrics});

  factory AiHealth.fromJson(Map<String, dynamic> json) {
    return AiHealth(
      status: json['status'] as String? ?? 'unknown',
      model: json['model'] as String? ?? '',
      uptime: json['uptime'] as int? ?? 0,
      metrics: json['metrics'] as Map<String, dynamic>? ?? {},
    );
  }
}
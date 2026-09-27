/// RadioSchedule represents the radio schedule
library models.src.radio_schedule;

import 'radio.dart';

/// RadioSchedule represents the radio schedule
class RadioSchedule {
  final List<RadioStation> stations;
  final List<ScheduledBlock> schedule;

  RadioSchedule({required this.stations, required this.schedule});

  factory RadioSchedule.fromJson(Map<String, dynamic> json) {
    return RadioSchedule(
      stations: (json['stations'] as List<dynamic>?)
              ?.map((e) => RadioStation.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
      schedule: (json['schedule'] as List<dynamic>?)
              ?.map((e) => ScheduledBlock.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'stations': stations.map((e) => e.toJson()).toList(),
      'schedule': schedule.map((e) => e.toJson()).toList(),
    };
  }
}

/// ScheduledBlock represents a scheduled block in the radio schedule
class ScheduledBlock {
  final String stationId;
  final int startTime;
  final int endTime;
  final String title;
  final String type;
  final String? description;

  ScheduledBlock({
    required this.stationId,
    required this.startTime,
    required this.endTime,
    required this.title,
    required this.type,
    this.description,
  });

  factory ScheduledBlock.fromJson(Map<String, dynamic> json) {
    return ScheduledBlock(
      stationId: json['station_id'] as String? ?? '',
      startTime: json['start_time'] as int? ?? 0,
      endTime: json['end_time'] as int? ?? 0,
      title: json['title'] as String? ?? '',
      type: json['type'] as String? ?? '',
      description: json['description'] as String?,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'station_id': stationId,
      'start_time': startTime,
      'end_time': endTime,
      'title': title,
      'type': type,
      'description': description,
    };
  }
}
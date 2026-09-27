import 'package:flutter/material.dart';
import 'package:models/models.dart';

/// RadioPlayerWidget manages radioplayerwidget functionality.
class RadioPlayerWidget extends StatefulWidget {
  final Future<List<RadioStation>> Function() fetchStations;
  final Future<List<RadioTrack>> Function(String stationId) fetchPlaylist;

  const RadioPlayerWidget({
    super.key,
    required this.fetchStations,
    required this.fetchPlaylist,
  });

  @override
  State<RadioPlayerWidget> createState() => _RadioPlayerWidgetState();
}

class _RadioPlayerWidgetState extends State<RadioPlayerWidget> {
  List<RadioStation> _stations = [];
  RadioStation? _selectedStation;
  List<RadioTrack> _playlist = [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _loadStations();
  }

  Future<void> _loadStations() async {
    setState(() => _loading = true);
    try {
      final stations = await widget.fetchStations();
      setState(() {
        _stations = stations;
        _loading = false;
        _error = null;
      });
    } catch (e) {
      setState(() {
        _loading = false;
        _error = e.toString();
      });
    }
  }

  Future<void> _selectStation(RadioStation station) async {
    setState(() {
      _selectedStation = station;
      _loading = true;
    });
    try {
      final playlist = await widget.fetchPlaylist(station.id);
      setState(() {
        _playlist = playlist;
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _loading = false;
        _error = e.toString();
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, size: 48, color: Colors.red),
            const SizedBox(height: 16),
            Text('Ошибка: $_error', textAlign: TextAlign.center),
            const SizedBox(height: 16),
            ElevatedButton(onPressed: _loadStations, child: const Text('Повторить')),
          ],
        ),
      );
    }
    if (_stations.isEmpty) {
      return const Center(child: Text('Нет доступных радиостанций'));
    }

    if (_selectedStation != null) {
      return _buildPlayerView();
    }
    return _buildStationList();
  }

  Widget _buildStationList() {
    return ListView.builder(
      itemCount: _stations.length,
      itemBuilder: (context, index) {
        final s = _stations[index];
        return Card(
          margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          child: ListTile(
            leading: Text(s.typeIcon + s.languageFlag, style: const TextStyle(fontSize: 28)),
            title: Text(s.name, style: const TextStyle(fontWeight: FontWeight.bold)),
            subtitle: Text('${s.languageLabel} • ${s.description}', maxLines: 2, overflow: TextOverflow.ellipsis),
            trailing: s.enabled
                ? const Icon(Icons.play_circle_fill, color: Colors.green)
                : const Icon(Icons.stop, color: Colors.grey),
            onTap: s.enabled ? () => _selectStation(s) : null,
          ),
        );
      },
    );
  }

  Widget _buildPlayerView() {
    return Column(
      children: [
        // Station header
        Container(
          padding: const EdgeInsets.all(16),
          color: Theme.of(context).colorScheme.primaryContainer,
          child: Row(
            children: [
              IconButton(
                icon: const Icon(Icons.arrow_back),
                onPressed: () => setState(() => _selectedStation = null),
              ),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(_selectedStation!.typeIcon + ' ' + _selectedStation!.name,
                        style: Theme.of(context).textTheme.titleLarge),
                    Text('${_selectedStation!.languageFlag} ${_selectedStation!.languageLabel}'),
                  ],
                ),
              ),
              // Placeholder: now playing indicator
              const Icon(Icons.radio, size: 40, color: Colors.green),
            ],
          ),
        ),
        // Playlist
        Expanded(
          child: _playlist.isEmpty
              ? const Center(child: Text('Плейлист пуст. Загрузите аудиофайлы через API.'))
              : ListView.builder(
                  itemCount: _playlist.length,
                  itemBuilder: (context, index) {
                    final t = _playlist[index];
                    return ListTile(
                      leading: Text(t.isAd ? '📢' : t.isAnnounce ? '📰' : '🎵', style: const TextStyle(fontSize: 24)),
                      title: Text(t.title, maxLines: 1, overflow: TextOverflow.ellipsis),
                      subtitle: Text('${t.typeLabel} • ${t.duration}с'),
                      dense: true,
                    );
                  },
                ),
        ),
      ],
    );
  }
}

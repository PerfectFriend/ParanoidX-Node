import 'client.dart';

/// RadioClient manages radioclient functionality.
class RadioClient {
  final SimplexNodeClient _client;
  RadioClient(this._client);

  Future<Map<String, dynamic>> stations({String? lang}) {
    final params = <String, String>{'action': 'stations'};
    if (lang != null) params['lang'] = lang;
    return _client.get('/api/radio', params: params);
  }

  Future<Map<String, dynamic>> playlist(String stationId) {
    return _client.get('/api/radio', params: {'action': 'playlist', 'station': stationId});
  }

  String trackUrl(String baseUrl, String stationId, String filename) {
    return '$baseUrl/api/vault/audio?path=$stationId/$filename';
  }
}

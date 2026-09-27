import 'dart:async';
import 'dart:io';
import 'dart:typed_data';
import 'package:http/http.dart' as http;

http.Client createHttpClient({bool useProxy = true, String socksHost = '127.0.0.1', int socksPort = 9050}) {
  if (!useProxy) return http.Client();
  return _TorAwareClient(socksHost: socksHost, socksPort: socksPort);
}

Future<bool> testOnionReachability(String host, int port,
    {String socksHost = '127.0.0.1', int socksPort = 9050, int timeoutMs = 8000}) async {
  try {
    final socket = await Socket.connect(socksHost, socksPort,
        timeout: Duration(milliseconds: timeoutMs));
    try {
      final it = StreamIterator(socket);
      socket.add(Uint8List.fromList([0x05, 0x01, 0x00]));
      await socket.flush();
      final hasAuth = await it.moveNext().timeout(const Duration(seconds: 5));
      if (!hasAuth || it.current.length < 2 || it.current[1] != 0x00) return false;
      final hostBytes = Uint8List.fromList(host.codeUnits);
      if (hostBytes.length > 255) return false;
      socket.add(Uint8List.fromList([
        0x05, 0x01, 0x00, 0x03,
        hostBytes.length,
        ...hostBytes,
        (port >> 8) & 0xff,
        port & 0xff,
      ]));
      await socket.flush();
      final hasConn = await it.moveNext().timeout(const Duration(seconds: 5));
      return hasConn && it.current.length >= 2 && it.current[1] == 0x00;
    } finally {
      await socket.close();
    }
  } catch (_) {
    return false;
  }
}

class _TorAwareClient extends http.BaseClient {
  final String _socksHost;
  final int _socksPort;
  HttpClient _directClient;

  _TorAwareClient({String socksHost = '127.0.0.1', int socksPort = 9050})
      : _socksHost = socksHost,
        _socksPort = socksPort,
        _directClient = HttpClient() {
    _directClient.badCertificateCallback = (_, __, ___) => true;
  }

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.url.host.endsWith('.onion')) {
      return _sendViaSocks5(request);
    }
    return _sendDirect(request);
  }

  Future<http.StreamedResponse> _sendDirect(http.BaseRequest request) async {
    var stream = request.finalize();
    try {
      var ioRequest = (await _directClient.openUrl(request.method, request.url))
        ..followRedirects = request.followRedirects
        ..maxRedirects = request.maxRedirects
        ..contentLength = (request.contentLength ?? -1)
        ..persistentConnection = request.persistentConnection;
      request.headers.forEach((name, value) {
        ioRequest.headers.set(name, value);
      });
      final response = await stream.pipe(ioRequest) as HttpClientResponse;
      var headers = <String, String>{};
      response.headers.forEach((key, values) {
        headers[key] = values.join(',');
      });
      return http.StreamedResponse(
        response,
        response.statusCode,
        contentLength: response.contentLength == -1 ? null : response.contentLength,
        headers: headers,
        isRedirect: response.isRedirect,
        persistentConnection: response.persistentConnection,
        reasonPhrase: response.reasonPhrase,
        request: request,
      );
    } on SocketException catch (error) {
      throw http.ClientException(error.message, request.url);
    } on HttpException catch (error) {
      throw http.ClientException(error.message, error.uri);
    }
  }

  Future<http.StreamedResponse> _sendViaSocks5(http.BaseRequest request) async {
    final url = request.url;
    final host = url.host;
    final port = url.port > 0 ? url.port : (url.scheme == 'https' ? 443 : 80);

    final socket = await Socket.connect(_socksHost, _socksPort,
        timeout: const Duration(seconds: 10));
    try {
      final it = StreamIterator(socket);

      // SOCKS5 auth
      socket.add(Uint8List.fromList([0x05, 0x01, 0x00]));
      await socket.flush();
      final hasAuth = await it.moveNext().timeout(const Duration(seconds: 5));
      if (!hasAuth || it.current.length < 2 || it.current[1] != 0x00) {
        throw Exception('SOCKS5 auth rejected');
      }

      // SOCKS5 connect
      final hostBytes = Uint8List.fromList(host.codeUnits);
      if (hostBytes.length > 255) throw Exception('Hostname too long');
      socket.add(Uint8List.fromList([
        0x05, 0x01, 0x00, 0x03,
        hostBytes.length,
        ...hostBytes,
        (port >> 8) & 0xff,
        port & 0xff,
      ]));
      await socket.flush();
      final hasConn = await it.moveNext().timeout(const Duration(seconds: 5));
      if (!hasConn || it.current.length < 2 || it.current[1] != 0x00) {
        throw Exception('SOCKS5 connect rejected');
      }

      // Build HTTP request
      final buf = StringBuffer();
      buf.writeln('${request.method} ${url.path}${url.query.isNotEmpty ? '?${url.query}' : ''} HTTP/1.1');
      buf.writeln('Host: ${url.host}${port != 80 && port != 443 ? ':$port' : ''}');
      request.headers.forEach((name, value) {
        // Skip headers we set manually
        final lower = name.toLowerCase();
        if (lower == 'host' || lower == 'content-length' || lower == 'connection') return;
        buf.writeln('$name: $value');
      });
      final bodyLength = request.contentLength ?? 0;
      if (bodyLength > 0) {
        buf.writeln('Content-Length: $bodyLength');
      }
      buf.writeln('Connection: close');
      buf.writeln('');

      // Send request
      socket.add(Uint8List.fromList(buf.toString().codeUnits));
      if (bodyLength > 0) {
        final bodyStream = request.finalize();
        await for (final chunk in bodyStream) {
          socket.add(chunk);
        }
      }
      await socket.flush();

      // Read response using same StreamIterator (single subscriber)
      final responseData = <int>[];
      while (await it.moveNext().timeout(const Duration(seconds: 10))) {
        responseData.addAll(it.current);
      }

      // Parse HTTP response
      final responseStr = String.fromCharCodes(responseData);
      final headerEnd = responseStr.indexOf('\r\n\r\n');
      if (headerEnd == -1) throw Exception('Invalid HTTP response');

      final statusLine = responseStr.substring(0, responseStr.indexOf('\r\n'));
      final statusParts = statusLine.split(' ');
      final statusCode = int.tryParse(statusParts[1]) ?? 500;
      final reasonPhrase = statusParts.length > 2 ? statusParts.sublist(2).join(' ') : '';

      // Parse headers
      final headerBlock = responseStr.substring(responseStr.indexOf('\r\n') + 2, headerEnd);
      final responseHeaders = <String, String>{};
      for (final line in headerBlock.split('\r\n')) {
        final colon = line.indexOf(':');
        if (colon > 0) {
          responseHeaders[line.substring(0, colon).trim().toLowerCase()] =
              line.substring(colon + 1).trim();
        }
      }

      // Extract body
      final bodyStart = headerEnd + 4;
      final bodyBytes = responseData.sublist(bodyStart);

      return http.StreamedResponse(
        Stream.value(bodyBytes),
        statusCode,
        contentLength: bodyBytes.length,
        headers: responseHeaders,
        reasonPhrase: reasonPhrase,
        request: request,
      );
    } finally {
      await socket.close();
    }
  }

  @override
  void close() {
    _directClient.close(force: true);
    super.close();
  }
}

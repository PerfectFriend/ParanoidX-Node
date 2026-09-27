import 'dart:io';
import 'package:flutter/material.dart';
import 'package:file_picker/file_picker.dart';
import 'package:path_provider/path_provider.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, VaultListing;

class VaultScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const VaultScreen({super.key, required this.client, required this.identity});

  @override
  State<VaultScreen> createState() => _VaultScreenState();
}

class _VaultScreenState extends State<VaultScreen> {
  VaultListing? _listing;
  bool _loading = true;
  bool _uploading = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final listing = await widget.client.vault.list();
      if (mounted) {
        setState(() {
          _listing = listing;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _loading = false);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Load failed: $e'), backgroundColor: Colors.red));
    }
  }

  Future<void> _uploadFile() async {
    final result = await FilePicker.platform.pickFiles();
    if (result == null || result.files.isEmpty) return;
    final file = result.files.first;
    final path = file.path;
    if (path == null) return;
    setState(() { _uploading = true; });
    try {
      await widget.client.vault.upload(path: path, name: file.name);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Uploaded ${file.name}'), backgroundColor: Colors.green));
      }
      await _load();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Upload failed: $e'), backgroundColor: Colors.red));
      }
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  Future<void> _downloadFile(String name) async {
    try {
      final bytes = await widget.client.vault.download(name);
      final dir = (await getApplicationDocumentsDirectory()).path;
      await Directory(dir).create(recursive: true);
      final file = File('$dir/$name');
      await file.writeAsBytes(bytes);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Downloaded $name'), backgroundColor: Colors.green));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Download failed: $e'), backgroundColor: Colors.red));
      }
    }
  }

  Future<void> _deleteFile(String name) async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Delete File'),
        content: Text('Permanently delete "$name"?'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          TextButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Delete', style: TextStyle(color: Colors.red))),
        ],
      ),
    );
    if (confirm != true) return;
    try {
      await widget.client.vault.delete(name);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Deleted $name'), backgroundColor: Colors.orange));
      }
      await _load();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Delete failed: $e'), backgroundColor: Colors.red));
      }
    }
  }

  IconData _iconForFile(String name) {
    final ext = name.split('.').last.toLowerCase();
    switch (ext) {
      case 'pdf': return Icons.picture_as_pdf;
      case 'jpg': case 'jpeg': case 'png': case 'gif': case 'webp': return Icons.image;
      case 'mp3': case 'wav': case 'ogg': case 'flac': return Icons.audiotrack;
      case 'mp4': case 'avi': case 'mkv': case 'mov': return Icons.videocam;
      case 'zip': case 'tar': case 'gz': case 'rar': case '7z': return Icons.folder_zip;
      case 'txt': case 'md': case 'json': case 'xml': case 'csv': return Icons.description;
      case 'doc': case 'docx': return Icons.article;
      default: return Icons.insert_drive_file;
    }
  }

  String _formatSize(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final usedMb = _listing?.files.fold(0.0, (sum, f) => sum + (f.size / (1024 * 1024))) ?? 0.0;
    const quotaMb = 2048.0;
    final usagePct = quotaMb > 0 ? (usedMb / quotaMb) : 0.0;
    final usageColor = usagePct > 0.9 ? Colors.red : (usagePct > 0.7 ? Colors.orange : Colors.green);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Vault'),
        actions: [
          if (_uploading)
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 12),
              child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
            )
          else
            IconButton(
              icon: const Icon(Icons.upload_file, size: 20),
              tooltip: 'Upload File',
              onPressed: _uploadFile,
            ),
          IconButton(icon: const Icon(Icons.refresh, size: 20), onPressed: _load),
        ],
      ),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Text('Storage', style: theme.textTheme.labelSmall),
                    const Spacer(),
                    Text('${usedMb.toStringAsFixed(1)} MB / ${quotaMb.toStringAsFixed(0)} MB', style: TextStyle(fontSize: 11, color: Colors.grey)),
                  ],
                ),
                const SizedBox(height: 4),
                ClipRRect(
                  borderRadius: BorderRadius.circular(4),
                  child: LinearProgressIndicator(
                    value: usagePct.clamp(0.0, 1.0),
                    backgroundColor: Colors.grey.shade200,
                    valueColor: AlwaysStoppedAnimation(usageColor),
                    minHeight: 8,
                  ),
                ),
              ],
            ),
          ),
          Expanded(
            child: _loading
                ? const Center(child: CircularProgressIndicator())
                : _listing?.files.isEmpty ?? true
                    ? const Center(child: Text('No files — tap upload to add one'))
                    : RefreshIndicator(
                        onRefresh: _load,
                        child: ListView.builder(
                          padding: const EdgeInsets.symmetric(horizontal: 12),
                          itemCount: _listing?.files.length ?? 0,
                          itemBuilder: (ctx, i) {
                            final f = _listing!.files[i];
                            return Card(
                              margin: const EdgeInsets.symmetric(vertical: 3),
                              child: ListTile(
                                leading: Icon(_iconForFile(f.name), color: theme.colorScheme.primary),
                                title: Text(f.name, overflow: TextOverflow.ellipsis),
                                subtitle: Text('${_formatSize(f.size)}  ${f.createdAt.isNotEmpty ? f.createdAt.substring(0, 10) : ''}', style: TextStyle(fontSize: 11, color: Colors.grey[600])),
                                trailing: Row(
                                  mainAxisSize: MainAxisSize.min,
                                  children: [
                                    IconButton(
                                      icon: const Icon(Icons.download, size: 20),
                                      tooltip: 'Download',
                                      onPressed: () => _downloadFile(f.name),
                                    ),
                                    IconButton(
                                      icon: const Icon(Icons.delete_outline, size: 20, color: Colors.red),
                                      tooltip: 'Delete',
                                      onPressed: () => _deleteFile(f.name),
                                    ),
                                  ],
                                ),
                              ),
                            );
                          },
                        ),
                      ),
          ),
        ],
      ),
    );
  }
}
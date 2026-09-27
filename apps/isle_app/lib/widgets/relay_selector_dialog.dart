import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import '../services/relay_manager.dart';

/// RelaySelectorDialog manages a dialog for selecting and testing SimpleX relay servers.
class RelaySelectorDialog extends StatefulWidget {
  final RelayManager manager;
  final http.Client httpClient;
  const RelaySelectorDialog({super.key, required this.manager, required this.httpClient});

  @override
  State<RelaySelectorDialog> createState() => _RelaySelectorDialogState();
}

class _RelaySelectorDialogState extends State<RelaySelectorDialog> {
  bool _testing = false;
  String? _selectedId;

  @override
  void initState() {
    super.initState();
    _selectedId = widget.manager.activeRelayId;
  }

  Future<void> _runTests() async {
    setState(() => _testing = true);
    await widget.manager.testAll(client: widget.httpClient);
    await widget.manager.save();
    if (mounted) setState(() => _testing = false);
  }

  @override
  Widget build(BuildContext context) {
    final relays = widget.manager.sortedByQuality;

    return AlertDialog(
      title: Row(
        children: [
          const Icon(Icons.settings_ethernet, size: 24),
          const SizedBox(width: 8),
          const Text('Select Relay'),
          const Spacer(),
          if (_testing)
            const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
        ],
      ),
      content: SizedBox(
        width: 480,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              'Relays are tested for connectivity and latency. '
              'Best quality relays appear first.',
              style: TextStyle(fontSize: 12, color: Colors.grey[600]),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                FilledButton.tonalIcon(
                  icon: _testing
                      ? const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2))
                      : const Icon(Icons.network_check, size: 16),
                  label: Text(_testing ? 'Testing...' : 'Test All Relays', style: const TextStyle(fontSize: 12)),
                  onPressed: _testing ? null : _runTests,
                ),
                const SizedBox(width: 8),
                OutlinedButton.icon(
                  icon: const Icon(Icons.add, size: 16),
                  label: const Text('Add Custom', style: TextStyle(fontSize: 12)),
                  onPressed: _addCustom,
                ),
              ],
            ),
            const SizedBox(height: 12),
            Expanded(
              child: relays.isEmpty
                  ? const Center(child: Text('No relays configured', style: TextStyle(color: Colors.grey)))
                  : ListView.builder(
                      itemCount: relays.length,
                      itemBuilder: (ctx, i) {
                        final r = relays[i];
                        final isSelected = r.id == _selectedId;
                        return Card(
                          color: isSelected ? Theme.of(context).colorScheme.primaryContainer : null,
                          child: ListTile(
                            dense: true,
                            leading: Radio<String>(
                              value: r.id,
                              groupValue: _selectedId,
                              onChanged: (v) {
                                setState(() => _selectedId = v);
                              },
                            ),
                            title: Row(
                              children: [
                                Text(r.label, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                                const SizedBox(width: 8),
                                if (r.reachable)
                                  Container(
                                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
                                    decoration: BoxDecoration(
                                      color: Colors.green.withAlpha(30),
                                      borderRadius: BorderRadius.circular(8),
                                    ),
                                    child: Text('${r.latencyMs}ms',
                                        style: const TextStyle(fontSize: 10, color: Colors.green)),
                                  )
                                else
                                  Container(
                                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
                                    decoration: BoxDecoration(
                                      color: Colors.grey.withAlpha(30),
                                      borderRadius: BorderRadius.circular(8),
                                    ),
                                    child: Text('offline',
                                        style: TextStyle(fontSize: 10, color: Colors.grey[600])),
                                  ),
                                if (isSelected)
                                  Container(
                                    margin: const EdgeInsets.only(left: 4),
                                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
                                    decoration: BoxDecoration(
                                      color: Colors.blue.withAlpha(30),
                                      borderRadius: BorderRadius.circular(8),
                                    ),
                                    child: const Text('active', style: TextStyle(fontSize: 10, color: Colors.blue)),
                                  ),
                              ],
                            ),
                            subtitle: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                if (r.hasSMP)
                                  Text('SMP: ${r.smp}', style: const TextStyle(fontSize: 9, fontFamily: 'monospace')),
                                if (r.hasXFTP)
                                  Text('XFTP: ${r.xftp}', style: const TextStyle(fontSize: 9, fontFamily: 'monospace')),
                              ],
                            ),
                            trailing: IconButton(
                              icon: const Icon(Icons.delete_outline, size: 16, color: Colors.red),
                              onPressed: () {
                                widget.manager.remove(r.id);
                                widget.manager.save();
                                setState(() {});
                              },
                            ),
                          ),
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(
          onPressed: () {
            if (_selectedId != null) {
              Navigator.pop(context, _selectedId);
            }
          },
          child: const Text('Select'),
        ),
      ],
    );
  }

  void _addCustom() {
    final idCtrl = TextEditingController();
    final smpCtrl = TextEditingController();
    final xftpCtrl = TextEditingController();
    final labelCtrl = TextEditingController();

    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Add Custom Relay'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(controller: labelCtrl, decoration: const InputDecoration(labelText: 'Label', border: OutlineInputBorder(), isDense: true)),
            const SizedBox(height: 8),
            TextField(controller: idCtrl, decoration: const InputDecoration(labelText: 'ID', border: OutlineInputBorder(), isDense: true)),
            const SizedBox(height: 8),
            TextField(controller: smpCtrl, decoration: const InputDecoration(labelText: 'SMP Address', border: OutlineInputBorder(), isDense: true)),
            const SizedBox(height: 8),
            TextField(controller: xftpCtrl, decoration: const InputDecoration(labelText: 'XFTP Address', border: OutlineInputBorder(), isDense: true)),
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(
            onPressed: () {
              if (idCtrl.text.trim().isNotEmpty) {
                widget.manager.addOrUpdate(RelayInfo(
                  id: idCtrl.text.trim(),
                  smp: smpCtrl.text.trim(),
                  xftp: xftpCtrl.text.trim(),
                  label: labelCtrl.text.trim().isNotEmpty ? labelCtrl.text.trim() : idCtrl.text.trim(),
                ));
                widget.manager.save();
                Navigator.pop(ctx);
                setState(() {});
              }
            },
            child: const Text('Add'),
          ),
        ],
      ),
    );
  }
}

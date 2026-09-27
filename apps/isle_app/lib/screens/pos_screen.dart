import 'package:flutter/material.dart';
import 'package:api_client/api_client.dart' show SimplexApiClient;
import 'package:models/models.dart' show Identity, Invoice;

class PosScreen extends StatefulWidget {
  final SimplexApiClient client;
  final Identity identity;

  const PosScreen({super.key, required this.client, required this.identity});

  @override
  State<PosScreen> createState() => _PosScreenState();
}

class _PosScreenState extends State<PosScreen> with SingleTickerProviderStateMixin {
  late TabController _tabs;
  List<Invoice> _invoices = [];
  bool _loading = false;
  String? _error;

  final _merchantCtrl = TextEditingController();
  final _amountCtrl = TextEditingController();
  final _descCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _tabs = TabController(length: 3, vsync: this);
    _loadInvoices();
  }

  @override
  void dispose() {
    _tabs.dispose();
    _merchantCtrl.dispose();
    _amountCtrl.dispose();
    _descCtrl.dispose();
    super.dispose();
  }

  Future<void> _loadInvoices() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final invoices = await widget.client.pos.listInvoices(pubkey: widget.identity.ed25519PubKey);
      if (mounted) {
        setState(() {
          _invoices = invoices;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _loading = false;
          _error = e.toString();
        });
      }
    }
  }

  Future<void> _createInvoice() async {
    final merchant = _merchantCtrl.text.trim();
    final amount = int.tryParse(_amountCtrl.text.trim());
    final desc = _descCtrl.text.trim();

    if (merchant.isEmpty || amount == null || amount <= 0) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Please enter merchant name and amount')),
      );
      return;
    }

    try {
      await widget.client.pos.createInvoice(
        merchant: merchant,
        amountNg: amount,
        description: desc.isEmpty ? null : desc,
      );
      _merchantCtrl.clear();
      _amountCtrl.clear();
      _descCtrl.clear();
      _loadInvoices();
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Invoice created')));
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Error: $e')));
    }
  }

  Future<void> _payInvoice(Invoice inv) async {
    try {
      await widget.client.pos.payInvoice(invoiceId: inv.id, pubkey: widget.identity.ed25519PubKey);
      _loadInvoices();
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Payment sent')));
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Error: $e')));
    }
  }

  String _formatNt(int ng) {
    return (ng / 1000000000).toStringAsFixed(9);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Point of Sale'),
        actions: [
          IconButton(icon: const Icon(Icons.add), onPressed: _createInvoice),
          IconButton(icon: const Icon(Icons.refresh), onPressed: _loadInvoices),
        ],
        bottom: TabBar(
          controller: _tabs,
          tabs: const [
            Tab(text: 'My Invoices'),
            Tab(text: 'Create'),
            Tab(text: 'QR Pay'),
          ],
        ),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(Icons.error_outline, size: 48, color: Colors.red),
                      const SizedBox(height: 16),
                      Text('Error: $_error'),
                      const SizedBox(height: 16),
                      FilledButton(onPressed: _loadInvoices, child: const Text('Retry')),
                    ],
                  ),
                )
              : TabBarView(
                  controller: _tabs,
                  children: [
                    // My Invoices
                    _invoices.isEmpty
                        ? const Center(child: Text('No invoices — tap + to create one'))
                        : RefreshIndicator(
                            onRefresh: _loadInvoices,
                            child: ListView.builder(
                              itemCount: _invoices.length,
                              itemBuilder: (ctx, i) {
                                final inv = _invoices[i];
                                return Card(
                                  margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                                  child: ListTile(
                                    leading: CircleAvatar(
                                      backgroundColor: inv.paid ? Colors.green.shade100 : Colors.amber.shade100,
                                      child: Icon(inv.paid ? Icons.check_circle : Icons.receipt_long, color: inv.paid ? Colors.green.shade700 : Colors.amber.shade700),
                                    ),
                                    title: Text('${inv.merchant}  ${_formatNt(inv.amountNg)} NT', style: const TextStyle(fontFamily: 'monospace')),
                                    subtitle: Text(inv.description ?? '', maxLines: 1, overflow: TextOverflow.ellipsis),
                                    trailing: !inv.paid
                                        ? FilledButton(
                                            onPressed: () => _payInvoice(inv),
                                            style: FilledButton.styleFrom(minimumSize: const Size(80, 36)),
                                            child: const Text('Pay'),
                                          )
                                        : Container(
                                            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                                            decoration: BoxDecoration(color: Colors.green.shade100, borderRadius: BorderRadius.circular(4)),
                                            child: Text('PAID', style: TextStyle(color: Colors.green.shade700, fontWeight: FontWeight.bold, fontSize: 12)),
                                          ),
                                  ),
                                );
                              },
                            ),
                          ),
                    // Create
                    Center(
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 400),
                        child: Card(
                          margin: const EdgeInsets.all(16),
                          child: Padding(
                            padding: const EdgeInsets.all(24),
                            child: Column(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                const Icon(Icons.receipt_long, size: 48, color: Colors.amber),
                                const SizedBox(height: 16),
                                const Text('Create Invoice', style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold)),
                                const SizedBox(height: 8),
                                const Text('Generate a QR invoice for in-person NT payments', textAlign: TextAlign.center),
                                const SizedBox(height: 24),
                                TextField(
                                  controller: _merchantCtrl,
                                  decoration: const InputDecoration(labelText: 'Merchant Name', border: OutlineInputBorder()),
                                ),
                                const SizedBox(height: 12),
                                TextField(
                                  controller: _amountCtrl,
                                  decoration: const InputDecoration(labelText: 'Amount (NT)', border: OutlineInputBorder()),
                                  keyboardType: TextInputType.number,
                                ),
                                const SizedBox(height: 12),
                                TextField(
                                  controller: _descCtrl,
                                  decoration: const InputDecoration(labelText: 'Description (optional)', border: OutlineInputBorder()),
                                ),
                                const SizedBox(height: 24),
                                FilledButton.icon(
                                  onPressed: _createInvoice,
                                  icon: const Icon(Icons.add),
                                  label: const Text('Create New Invoice'),
                                  style: FilledButton.styleFrom(minimumSize: const Size(double.infinity, 48)),
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                    // QR Pay
                    Center(
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 400),
                        child: Card(
                          margin: const EdgeInsets.all(16),
                          child: Padding(
                            padding: const EdgeInsets.all(24),
                            child: Column(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                const Icon(Icons.qr_code_scanner, size: 48, color: Colors.blue),
                                const SizedBox(height: 16),
                                const Text('Scan QR to Pay', style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold)),
                                const SizedBox(height: 8),
                                const Text('Point camera at merchant QR code to pay invoice', textAlign: TextAlign.center),
                                const SizedBox(height: 24),
                                FilledButton.icon(
                                  onPressed: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('QR scanner not implemented yet'))),
                                  icon: const Icon(Icons.camera_alt),
                                  label: const Text('Open Scanner'),
                                  style: FilledButton.styleFrom(minimumSize: const Size(double.infinity, 48)),
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
    );
  }
}
import 'package:flutter/material.dart';

/// ChatInputBar manages the chat input bar with text field, templates, attachments and send controls.
class ChatInputBar extends StatelessWidget {
  final TextEditingController controller;
  final bool hasScheduledTime;
  final String? replyingTo;
  final String? replyingText;
  final bool showTemplatePicker;
  final bool showSuggestions;
  final List<String> suggestions;
  final VoidCallback? onSend;
  final VoidCallback? onAttachFile;
  final VoidCallback? onVoice;
  final VoidCallback? onAI;
  final VoidCallback? onTranslate;
  final VoidCallback? onShowTemplates;
  final VoidCallback? onShowSchedule;
  final VoidCallback? onDismissReply;

  const ChatInputBar({
    super.key,
    required this.controller,
    this.hasScheduledTime = false,
    this.replyingTo,
    this.replyingText,
    this.showTemplatePicker = false,
    this.showSuggestions = false,
    this.suggestions = const [],
    this.onSend,
    this.onAttachFile,
    this.onVoice,
    this.onAI,
    this.onTranslate,
    this.onShowTemplates,
    this.onShowSchedule,
    this.onDismissReply,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(color: Colors.grey.shade900, border: Border(top: BorderSide(color: Colors.grey.shade800))),
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        if (replyingTo != null)
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            color: Colors.blue.shade900.withValues(alpha: 0.3),
            child: Row(children: [
              Icon(Icons.reply, size: 16, color: Colors.blue[300]),
              const SizedBox(width: 6),
              Expanded(child: Text(replyingText ?? '', style: TextStyle(fontSize: 14, color: Colors.grey[400]), maxLines: 1, overflow: TextOverflow.ellipsis)),
              IconButton(icon: const Icon(Icons.close, size: 18), onPressed: onDismissReply, padding: EdgeInsets.zero, constraints: const BoxConstraints()),
            ]),
          ),
        if (showSuggestions && suggestions.isNotEmpty)
          Container(
            height: 40,
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: ListView.builder(
              scrollDirection: Axis.horizontal,
              itemCount: suggestions.length,
              itemBuilder: (_, i) => Padding(
                padding: const EdgeInsets.only(right: 6),
                child: ActionChip(
                  label: Text(suggestions[i], style: const TextStyle(fontSize: 14)),
                  onPressed: () => controller.text = suggestions[i],
                  backgroundColor: Colors.grey.shade800,
                ),
              ),
            ),
          ),
        if (showTemplatePicker)
          Container(
            height: 40,
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: const Center(child: Text('Select a template from settings', style: TextStyle(fontSize: 14, color: Colors.grey))),
          ),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
          child: Row(children: [
            IconButton(icon: const Icon(Icons.description, size: 22), onPressed: onShowTemplates, tooltip: 'Templates', color: Colors.grey[400]),
            IconButton(icon: const Icon(Icons.attach_file, size: 22), onPressed: onAttachFile, tooltip: 'Attach', color: Colors.grey[400]),
            IconButton(icon: const Icon(Icons.mic, size: 22), onPressed: onVoice, tooltip: 'Voice', color: Colors.grey[400]),
            IconButton(icon: const Icon(Icons.smart_toy, size: 22), onPressed: onAI, tooltip: 'AI', color: Colors.cyan),
            IconButton(icon: const Icon(Icons.translate, size: 22), onPressed: onTranslate, tooltip: 'Translate', color: Colors.grey[400]),
            IconButton(
              icon: Icon(Icons.schedule, size: 22, color: hasScheduledTime ? Colors.amber : Colors.grey[400]),
              onPressed: onShowSchedule, tooltip: hasScheduledTime ? 'Scheduled' : 'Schedule',
            ),
            Expanded(
              child: TextField(
                controller: controller,
                style: const TextStyle(fontSize: 17, color: Colors.white),
                decoration: InputDecoration(
                  hintText: 'Message...',
                  hintStyle: TextStyle(fontSize: 17, color: Colors.grey[600]),
                  border: OutlineInputBorder(borderRadius: BorderRadius.circular(20), borderSide: BorderSide.none),
                  filled: true, fillColor: Colors.grey.shade800,
                  contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                ),
                textInputAction: TextInputAction.send,
                onSubmitted: (_) => onSend?.call(),
              ),
            ),
            const SizedBox(width: 4),
            IconButton(
              icon: Icon(Icons.send, size: 22, color: Theme.of(context).colorScheme.primary),
              onPressed: onSend,
              tooltip: 'Send',
            ),
          ]),
        ),
      ]),
    );
  }
}

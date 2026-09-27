import 'package:flutter_test/flutter_test.dart';
import 'package:royal_app/main.dart';

void main() {
  testWidgets('Royal app smoke test', (WidgetTester tester) async {
    await tester.pumpWidget(const RoyalApp());
    expect(find.text('Isle Royal Dashboard'), findsWidgets);
  });
}

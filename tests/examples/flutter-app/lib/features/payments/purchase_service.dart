import 'package:in_app_purchase/in_app_purchase.dart';

class PurchaseService {
  final InAppPurchase _store = InAppPurchase.instance;

  Future<bool> buyPremium() async {
    final response = await _store.queryProductDetails({'premium'});
    if (response.productDetails.isEmpty) return false;
    return _store.buyNonConsumable(
      purchaseParam: PurchaseParam(productDetails: response.productDetails.first),
    );
  }
}

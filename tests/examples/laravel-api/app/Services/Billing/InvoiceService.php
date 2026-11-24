<?php

namespace App\Services\Billing;

use App\Models\Order;

class InvoiceService
{
    public function createFor(Order $order): array
    {
        return [
            'order_id' => $order->id,
            'net_cents' => $order->total_cents,
            'tax_cents' => (int) round($order->total_cents * 0.19),
        ];
    }
}

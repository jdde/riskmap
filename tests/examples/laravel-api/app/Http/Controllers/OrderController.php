<?php

namespace App\Http\Controllers;

use App\Models\Order;
use App\Services\Billing\InvoiceService;
use Illuminate\Http\Request;

class OrderController extends Controller
{
    public function index(Request $request)
    {
        return $request->user()->orders()->latest()->get();
    }

    public function store(Request $request, InvoiceService $invoices)
    {
        $order = $request->user()->orders()->create(
            $request->validate(['total_cents' => ['required', 'integer', 'min:1']])
        );

        $invoices->createFor($order);

        return response()->json($order, 201);
    }

    public function show(Order $order)
    {
        $this->authorize('view', $order);

        return $order;
    }
}

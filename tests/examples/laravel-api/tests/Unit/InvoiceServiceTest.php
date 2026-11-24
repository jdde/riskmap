<?php

namespace Tests\Unit;

use App\Models\Order;
use App\Services\Billing\InvoiceService;
use PHPUnit\Framework\TestCase;

class InvoiceServiceTest extends TestCase
{
    public function test_it_adds_tax(): void
    {
        $invoice = (new InvoiceService)->createFor(new Order(['total_cents' => 1000]));

        $this->assertSame(190, $invoice['tax_cents']);
    }
}

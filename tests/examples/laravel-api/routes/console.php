<?php

use Illuminate\Support\Facades\Artisan;

Artisan::command('orders:count', function () {
    $this->info((string) \App\Models\Order::count());
});

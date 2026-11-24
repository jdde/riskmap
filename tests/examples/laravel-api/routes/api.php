<?php

use App\Http\Controllers\Auth\LoginController;
use App\Http\Controllers\OrderController;
use App\Http\Middleware\EnsureTenant;
use Illuminate\Support\Facades\Route;

Route::post('/login', LoginController::class);

Route::middleware(['auth:sanctum', EnsureTenant::class])->group(function () {
    Route::apiResource('orders', OrderController::class)->only(['index', 'store', 'show']);
});

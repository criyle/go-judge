#include <chrono>
#include <cstdint>
#include <iostream>
#include <string>

static void burn(std::chrono::milliseconds duration) {
    const auto end = std::chrono::steady_clock::now() + duration;
    volatile std::uint64_t value = 1;
    while (std::chrono::steady_clock::now() < end) {
        value = value * 1664525 + 1013904223;
    }
}

int main() {
    std::string command;
    while (std::getline(std::cin, command)) {
        if (command == "normal") {
            burn(std::chrono::milliseconds(10));
            std::cout << "MOVE normal\n" << std::flush;
        } else if (command == "move-timeout") {
            burn(std::chrono::milliseconds(300));
            std::cout << "MOVE late\n" << std::flush;
        } else if (command == "total-step") {
            burn(std::chrono::milliseconds(80));
            std::cout << "MOVE cumulative\n" << std::flush;
        }
    }
}


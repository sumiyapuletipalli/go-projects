package main

import "fmt"

func main() {
	for {
		var num int
		fmt.Print("Enter a number: ")
		fmt.Scan(&num)
		if num <= 0 {
			fmt.Println("Please enter a positive integer.")
			return
		}

		var factors []int
		count := 0
		for i := 1; i <= num; i++ {
			if num%i == 0 {
				factors = append(factors, i)
				count++
			}
		}

		fmt.Printf("Factors of %d: %v\n", num, factors)
		fmt.Printf("Number of factors for the given number %d is:", count)
		if count == 2 {
			fmt.Println("The number is Prime Number")

		}
	}
}

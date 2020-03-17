import math, csv
import random
import os, binascii

A = -1
D = 79743

F = GF(258664426012969094010652733694893533536393512754914660539884262666720468348340822774968888139573360124440321458177)

a = F(2*(A + D)/(A - D))
b = F(4/(A - D))


def add(a, b, x1, y1, x2, y2):
	x3 = b*((y2 - y1)**(2))/((x2 - x1)**(2))- a - x1 - x2
	y3 = (2*x1 + x2 + a) * (y2 - y1)/(x2 - x1) - b*((y2 - y1)**(3))/((x2 - x1)**(3)) - y1

	return(x3, y3)

def double(a, b, x1, y1):
	x3 = b*((3*(x1**2) + 2*a*x1 +1 )**2)/((2*b*y1)**2) -a -x1 - x1
	y3 = (2*x1 + x1 + a) * (3*(x1)**(2) + 2*a*x1 + 1)/(2*b*y1) - b*((3*(x1**(2))+2*a*x1+1)**3)/((2*b*y1)**3)-y1

	return x3, y3

def scalar_mult(a, b, s, x1, y1):
    bits = Integer(s).digits(2)[::-1]
    x, y = F(0), F(1)
    for b in bits:
        x, y = double(a, d, x, y)
        if b == 1:
            x, y = add(a, d, x, y, x1, y1)
    return(x, y)

#corresponding point on Montgomery curve
def edToMont(x, y):
	u = (1 + y)/(1 - y)
	v = (1 + y)/(x*(1-y))

	return(u, v)

#corresponding point on Edwards curve
def montToEd(u, v):
	x = u/v
	y = (u - 1)/(u + 1)

	return(x, y)

def isOnCurveMont(a, b, u, v):
	return(b*v**2== u**3 + a*u**2 + u)

def isOnCurveEd(a, d, x, y):
	return(a*x**2 + y**2 == 1 + d*(x**2)*(y**2))

gx = F(174701772324485506941690903512423551998294352968833659960042362742684869862495746426366187462669992073196420267127)
gy = F(208487200052258845495340374451540775445408439654930191324011635560142523886549663106522691296420655144190624954833)

doub = double(a, b, edToMont(gx, gy)[0], edToMont(gx, gy)[1] )
t = montToEd( doub[0], doub[1])
print(t)
